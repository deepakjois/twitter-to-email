package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/peterbourgon/ff"
)

var (
	// Configuration
	bucket,
	consumer_api_key,
	consumer_api_secret_key,
	access_token,
	access_token_secret,
	email *string

	sess = session.Must(session.NewSession())
)

// formatDate formats dates into a valid S3 key
func formatDate(date time.Time) string {
	return fmt.Sprintf("tweets/%d-%02d-%02d/tweets.json", date.Year(), date.Month(), date.Day())
}

// getTodaysKey returns a valid key name derived from the current date in UTC
func getTodaysKey() string {
	return formatDate(time.Now().UTC())
}

// getYesterdaysKey returns a valid key name derived from the previous day in UTC
func getYesterdaysKey() string {
	return formatDate(time.Now().UTC().AddDate(0, 0, -1))
}

// getStoredTweets retrieves stored tweets from a given key in the S3 bucket
func getStoredTweets(key string) ([]twitter.Tweet, error) {
	svc := s3.New(sess)
	fmt.Printf("Getting tweets from: s3://%s/%s\n", *bucket, key)
	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: bucket,
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, err
	}

	var tweets []twitter.Tweet
	err = json.NewDecoder(result.Body).Decode(&tweets)
	return tweets, err
}

// uploadTweets uploads tweets into S3 bucket at given key
func uploadTweets(key string, tweets []twitter.Tweet) error {
	uploader := s3manager.NewUploader(sess)
	buf := bytes.NewBuffer([]byte{})
	err := json.NewEncoder(buf).Encode(tweets)
	if err != nil {
		return err
	}

	fmt.Printf("Uploading %d tweets to s3://%s/%s\n", len(tweets), *bucket, key)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: bucket,
		Key:    aws.String(key),
		Body:   buf,
	})

	if err != nil {
		return err
	}
	return nil
}

// getNewTweets retrieves tweets newer than sinceID using the Twitter API
func getNewTweets(sinceID int64) ([]twitter.Tweet, error) {
	config := oauth1.NewConfig(*consumer_api_key, *consumer_api_secret_key)
	token := oauth1.NewToken(*access_token, *access_token_secret)
	// OAuth1 http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client := twitter.NewClient(httpClient)

	// Home Timeline
	homeTimelineParams := &twitter.HomeTimelineParams{
		SinceID:   sinceID,
		TweetMode: "extended",
		Count:     200,
	}
	tweets, _, err := client.Timelines.HomeTimeline(homeTimelineParams)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%d New Tweets Found\n", len(tweets))

	return tweets, nil
}

// TODO document this
func fetchTweets() error {
	today := getTodaysKey()
	storedTweets, err := getStoredTweets(today)

	var sinceID int64
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchKey:
				fmt.Printf("%s not found. Trying to retrieve yesterday’s tweets\n", today)
				yesterday := getYesterdaysKey()
				storedTweets, err := getStoredTweets(yesterday)
				if err != nil {
					if aerr, ok := err.(awserr.Error); ok {
						switch aerr.Code() {
						case s3.ErrCodeNoSuchKey:
							fmt.Printf("%s not found.\n", yesterday)
						default:
							return aerr
						}
					} else {
						return err
					}
				}

				if len(storedTweets) > 0 {
					fmt.Println("Emailing yesterday’s tweets")
					err = emailTweets(storedTweets)
					if err != nil {
						return err
					}

					// Find last tweet from yesterday
					lastTweet := storedTweets[0]
					for _, tweet := range storedTweets {
						if tweet.ID > lastTweet.ID {
							lastTweet = tweet
						}
					}

					sinceID = lastTweet.ID

					storedTweets = []twitter.Tweet{lastTweet}
					fmt.Println("Uploading last tweet from yesterday for tracking")
				} else {
					fmt.Printf("Uploading an empty array to %s\n", today)
				}

				err = uploadTweets(today, storedTweets)
				if err != nil {
					return err
				}
			default:
				return aerr
			}
		} else {
			return aerr
		}
	} else {
		fmt.Printf("%d Older Tweets Found\n", len(storedTweets))

		for _, tweet := range storedTweets {
			if tweet.ID > sinceID {
				sinceID = tweet.ID
			}
		}
	}

	newTweets, err := getNewTweets(sinceID)

	if err != nil {
		return err
	}

	if len(newTweets) == 0 {
		// Nothing more to do
		return nil
	}

	tweets := append(newTweets, storedTweets...)

	return uploadTweets(today, tweets)
}

// emailTweets formats and emails tweets
func emailTweets(tweets []twitter.Tweet) error {
	builder := strings.Builder{}

	for i := len(tweets) - 1; i > -1; i-- {
		tweet := tweets[i]
		builder.WriteString(fmt.Sprintf("@%s: %s\nhttps://twitter.com/%s/status/%d\n\n--\n", tweet.User.ScreenName, tweet.FullText, tweet.User.ScreenName, tweet.ID))
	}

	svc := ses.New(session.Must(session.NewSession(&aws.Config{
		Region: aws.String("us-east-1")}, // SES is only available in limited AWS regions, so we hardcode the region here.
	)))

	// Assemble the email.
	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			CcAddresses: []*string{},
			ToAddresses: []*string{
				email,
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Text: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(builder.String()),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String(fmt.Sprintf("Tweets from the past 24h (%d)", len(tweets))),
			},
		},
		Source: email,
	}

	// Attempt to send the email.
	_, err := svc.SendEmail(input)
	return err
}

// getConfig populates the config variables from a JSON file
func getConfig() {
	fs := flag.NewFlagSet("twitter-to-email", flag.ExitOnError)

	bucket = fs.String("bucket", "", "S3 Bucket")
	consumer_api_key = fs.String("consumer-api-key", "", "Twitter Consumer API Key")
	consumer_api_secret_key = fs.String("consumer-api-secret-key", "", "Twitter Consumer API Secret Key")
	access_token = fs.String("access-token", "", "Twitter Access token")
	access_token_secret = fs.String("access-token-secret", "", "Twitter Access token secret")
	email = fs.String("email", "", "Email")

	ff.Parse(fs, []string{},
		ff.WithConfigFile("config.json"),
		ff.WithConfigFileParser(ff.JSONParser))
}

func main() {
	getConfig()
	lambda.Start(fetchTweets)
}
