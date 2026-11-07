# အခြေခံ OS အနေနဲ့ Alpine ကိုသုံးမယ် (ပေါ့ပါးလို့ပါ)
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Go Code ကို Build လုပ်မယ်
RUN go build -o main .

# Final Image ဖန်တီးမယ်
FROM alpine:latest
# 🌟 ဒီနေရာက အသက်ပါ။ FFmpeg ကို ဆာဗာထဲ အလိုအလျောက် သွင်းပေးသွားပါမယ်။
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /app/main .
# Port 8080 ကို ဖွင့်မယ်
EXPOSE 8080
CMD ["./main"]