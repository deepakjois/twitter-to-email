#!/bin/sh
GOARCH=amd64 GOOS=linux go build
zip -o twitter-to-email.zip twitter-to-email config.json
