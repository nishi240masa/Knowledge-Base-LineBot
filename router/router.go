package router

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/ikawaha/kagome-dict/ipa"
	kagome "github.com/ikawaha/kagome/v2/tokenizer"
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

var (
	bot          *linebot.Client
	sheetService *sheets.Service
	sheetID      = os.Getenv("GOOGLE_SHEET_ID")
)

// ===== 類義語辞書（拡張したいキーワードをマッピング） =====
var synonymMap = map[string][]string{
	"画像":      {"写真", "スクリーンショット", "スクショ"},
	"名前":      {"氏名", "フルネーム", "よびかた"},
	"趣味":      {"好きなこと", "興味", "遊び"},
	"年齢":      {"生まれた年", "誕生日", "年数"},
	"好きな言葉":   {"好きなフレーズ", "好きなセリフ", "好きなことば"},
	"吸ってるタバコ": {"タバコ", "煙草", "喫煙"},
}

// ===== 形態素解析して意味のある語（名詞・動詞・形容詞）を抽出 =====
func extractKeywords(text string) []string {
	t, err := kagome.New(ipa.Dict(), kagome.OmitBosEos())
	if err != nil {
		log.Printf("Tokenizer error: %v", err)
		return nil
	}
	tokens := t.Tokenize(text)

	var keywords []string
	for _, token := range tokens {
		if token.Class == kagome.DUMMY {
			continue
		}
		features := token.Features()
		if len(features) > 0 {
			pos := features[0]
			if pos == "名詞" || pos == "動詞" || pos == "形容詞" {
				keywords = append(keywords, token.Surface)
			}
		}
	}
	return keywords
}

// ===== 類義語展開 =====
func expandSynonyms(word string) []string {
	expanded := []string{word}
	if syns, ok := synonymMap[word]; ok {
		expanded = append(expanded, syns...)
	}
	return expanded
}

// ===== 初期化関数 =====
func Init() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	r := gin.Default()

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
		MaxAge:       12 * time.Hour,
	}))

	// LINE bot
	var err error
	bot, err = linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
	)
	if err != nil {
		log.Fatalf("LINE Bot init error: %v", err)
	}

	// Sheets API
	ctx := context.Background()
	sheetService, err = sheets.NewService(ctx, option.WithCredentialsJSON([]byte(os.Getenv("GOOGLE_CREDENTIALS_JSON"))))
	if err != nil {
		log.Fatalf("Sheets API init error: %v", err)
	}

	// ヘルスチェック
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "connection success"})
	})

	// Webhook
	r.POST("/callback", handleCallback)

	// 起動
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// ===== LINEのWebhookハンドラー =====
func handleCallback(c *gin.Context) {
	events, err := bot.ParseRequest(c.Request)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			c.AbortWithStatus(http.StatusBadRequest)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			if msg, ok := event.Message.(*linebot.TextMessage); ok {
				answer := findAnswerFromSheets(msg.Text)
				if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(answer)).Do(); err != nil {
					log.Printf("Reply error: %v", err)
				}
			}
		}
	}
	c.Status(http.StatusOK)
}

// ===== Google Sheetsからマッチング回答を返す =====
func findAnswerFromSheets(question string) string {
	resp, err := sheetService.Spreadsheets.Values.Get(sheetID, "FAQ!A:B").Do()
	if err != nil {
		log.Printf("Error reading sheet: %v", err)
		return "エラーが発生しました。"
	}

	tokens := extractKeywords(question)
	if len(tokens) == 0 {
		return "ご質問の意図がうまく読み取れませんでした。"
	}

	// 1件ずつFAQ行と比較
	for _, row := range resp.Values {
		if len(row) < 2 {
			continue
		}
		keywordText := fmt.Sprintf("%v", row[0])
		answer := fmt.Sprintf("%v", row[1])
		keywordWords := strings.Fields(strings.ToLower(keywordText))

		for _, token := range tokens {
			log.Printf("token: %v", token)
			for _, keyword := range keywordWords {
				for _, synonym := range expandSynonyms(keyword) {
					if strings.Contains(strings.ToLower(token), synonym) {
						return answer
					}
				}
			}
		}
	}
	return "その質問にはまだ対応していません。"
}
