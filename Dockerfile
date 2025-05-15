# Goの公式イメージを使用
FROM golang:1.24.2-alpine

# 必要なツールをインストール
RUN apk update && apk add --no-cache git

# 作業ディレクトリを設定
WORKDIR /go/src/app/

# go.mod と go.sum をコピーして依存関係をダウンロード
COPY ./go.mod ./go.sum ./
RUN go mod download

# アプリケーションコードをコピー
COPY ./ ./

# アプリケーションのビルド
RUN go build -o main .

# RenderのPORT環境変数を使用
ENV PORT=8080
EXPOSE 8080

# アプリケーションを起動
CMD ["./main"]