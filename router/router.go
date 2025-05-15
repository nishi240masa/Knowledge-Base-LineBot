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
	"github.com/line/line-bot-sdk-go/v7/linebot"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

var (
	bot          *linebot.Client
	sheetService *sheets.Service
	sheetID      = os.Getenv("GOOGLE_SHEET_ID")
)

func Init() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	// Ginルーター初期化
	r := gin.Default()

	// CORS設定
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
		MaxAge:       12 * time.Hour,
	}))

	// LINE Bot初期化
	var err error
	bot, err = linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
	)
	if err != nil {
		log.Fatalf("LINE Bot init error: %v", err)
	}

	// Sheets API 初期化
	ctx := context.Background()
	sheetService, err = sheets.NewService(ctx, option.WithCredentialsJSON([]byte(os.Getenv("GOOGLE_CREDENTIALS_JSON"))))
	if err != nil {
		log.Fatalf("Sheets API init error: %v", err)
	}

	// 接続確認用エンドポイント
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "connection success"})
	})

	// LINEのWebhook
	r.POST("/callback", handleCallback)

	// サーバ起動
	if err := r.Run(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

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

// Google SheetsからFAQを取得して質問に合う答えを返す
func findAnswerFromSheets(question string) string {
	//　質問の取得
	resp, err := sheetService.Spreadsheets.Values.Get(sheetID, "FAQ!A").Do()
	if err != nil {
		log.Printf("Failed to read spreadsheet: %v", err)
		return "エラーが発生しました（FAQ読み込み失敗）。"
	}

	//　回答の取得
	resp2, err := sheetService.Spreadsheets.Values.Get(sheetID, "FAQ!B").Do()
	if err != nil {
		log.Printf("Failed to read spreadsheet: %v", err)
		return "エラーが発生しました（FAQ読み込み失敗）。"
	}

	// 小文字化して比較（大文字小文字を無視するため）
	q := strings.ToLower(question)

	for i, row := range resp.Values {
		if len(row) > 0 {
			//　質問の取得
			questionFromSheet := strings.ToLower(row[0].(string))
			if strings.Contains(questionFromSheet, q) {
				//　回答の取得
				answer := resp2.Values[i][0].(string)
				return answer
			}
		}
	}

	return "その質問にはまだ対応していません。"
}
