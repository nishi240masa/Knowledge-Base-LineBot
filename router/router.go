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
	sheetID      = os.Getenv("GOOGLE_SHEET_ID") // Google Sheets ID
)

func Init() {
	// 環境変数 PORT を取得
	port := os.Getenv("PORT")
	if port == "" {
		port = "80" // デフォルトポート
	}

	// ルーターの初期化
	r := gin.Default()

	var err error

	// LINE Bot 初期化
	bot, err = linebot.New(
		// LINEのチャンネルシークレット
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Google Sheets API 初期化
	ctx := context.Background()
	// Google Sheets APIの認証情報をJSON形式で取得
	sheetService, err = sheets.NewService(ctx, option.WithCredentialsJSON([]byte(os.Getenv("GOOGLE_CREDENTIALS_JSON"))))
	if err != nil {
		log.Fatalf("Unable to create Sheets service: %v", err)
	}

	// CORSの設定
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},                                                                                                           // 許可するオリジン
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},                                                                     // 許可するHTTPメソッド
		AllowHeaders: []string{"Access-Control-Allow-Credentials", "Access-Control-Allow-Headers", "Origin", "Content-Type", "Authorization"}, // 許可するヘッダー
		MaxAge:       12 * time.Hour,                                                                                                          // キャッシュの最大時間
	}))

	// connectionTest
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "connection success!!!!",
		})
	})

	// Webhookエンドポイント

	r.POST("/callback", handleCallback)

	// 指定されたポートでサーバーを開始
	if err := r.Run(fmt.Sprintf(":%s", port)); err != nil {
		fmt.Printf("Failed to start server: %s\n", err)
	}
}

func handleCallback(c *gin.Context) {
	events, err := bot.ParseRequest(c.Request)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			c.Status(http.StatusBadRequest)
		} else {
			c.Status(http.StatusInternalServerError)
		}
		return
	}

	// Google SheetsからFAQを取得
	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			if msg, ok := event.Message.(*linebot.TextMessage); ok {
				answer := findAnswer(msg.Text)
				// ユーザーに返信
				if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(answer)).Do(); err != nil {
					log.Print(err)
				}
			}
		}
	}
	c.Status(http.StatusOK)
}

func findAnswer(question string) string {
	// Google SheetsからFAQを取得
	resp, err := sheetService.Spreadsheets.Values.Get(sheetID, "FAQ!A:B").Do()
	if err != nil {
		log.Printf("Error reading sheet: %v", err)
		return "エラーが発生しました。"
	}

	for _, row := range resp.Values {
		if len(row) >= 2 {
			if strings.Contains(question, row[0].(string)) {
				return row[1].(string)
			}
		}
	}
	return "その質問にはまだ対応していません。"
}
