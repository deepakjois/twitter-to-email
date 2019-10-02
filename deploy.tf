# Deploying to AWS
provider "aws" {
}

# Terraform state in S3
terraform {
  backend "s3" {
    bucket = "twitter-to-email-debugjois"
    key    = "terraform/terraform.tfstate"
  }
}

# S3 bucket to store tweets
# FIXME: Change this to a unique value
locals {
  twitter_to_email_bucket = "twitter-to-email-debugjois"
}


# IAM Role for Lambda function.
# We will attach policies to it below
resource "aws_iam_role" "twitter_to_email_iam_role" {
  name = "TwitterToEmailRole"
  path = "/service-role/"
  assume_role_policy = jsonencode(
    {
      Statement = [
        {
          Action = "sts:AssumeRole"
          Effect = "Allow"
          Principal = {
            Service = "lambda.amazonaws.com"
          }
        },
      ]
      Version = "2012-10-17"
    }
  )
}

# Basic execution policy
data "aws_iam_policy" "AWSLambdaBasicExecutionRole" {
  arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Policy to enable full read/write access to S3
# TODO: We should scope this down to a specific bucket
data "aws_iam_policy" "AmazonS3FullAccess" {
  arn = "arn:aws:iam::aws:policy/AmazonS3FullAccess"
}

# Policy to allow email access
# TODO: We should scope this down to the specific APIs
data "aws_iam_policy" "AmazonSESFullAccess" {
  arn = "arn:aws:iam::aws:policy/AmazonSESFullAccess"
}

# Attach policies to role

resource "aws_iam_role_policy_attachment" "lambda_basic_policy" {
  role       = aws_iam_role.twitter_to_email_iam_role.name
  policy_arn = data.aws_iam_policy.AWSLambdaBasicExecutionRole.arn
}
resource "aws_iam_role_policy_attachment" "s3_access_policy" {
  role       = aws_iam_role.twitter_to_email_iam_role.name
  policy_arn = data.aws_iam_policy.AmazonS3FullAccess.arn
}
resource "aws_iam_role_policy_attachment" "ses_access_policy" {
  role       = aws_iam_role.twitter_to_email_iam_role.name
  policy_arn = data.aws_iam_policy.AmazonSESFullAccess.arn
}


# Lambda function to fetch tweets periodically, store them in
# S3, and email them as a digest every 24hrs
resource "aws_lambda_function" "twitter_to_email" {

  function_name = "TwitterToEmailFn"
  description   = "Lambda handler to fetch tweets, store them in S3 and email them"
  handler       = "twitter-to-email"
  runtime       = "go1.x"

  filename         = "twitter-to-email.zip"
  source_code_hash = filebase64sha256("twitter-to-email.zip")

  role        = aws_iam_role.twitter_to_email_iam_role.arn
  memory_size = 256
  timeout     = 15
  publish     = true
}


# Rules and permissions to call TwitterToEmailFn every 5m
resource "aws_cloudwatch_event_rule" "every_five_minutes" {
  name                = "every-five-minutes"
  description         = "Fires every five minutes"
  schedule_expression = "rate(5 minutes)"
}
resource "aws_cloudwatch_event_target" "twitter_to_email_every_five_minutes" {
  rule      = "${aws_cloudwatch_event_rule.every_five_minutes.name}"
  target_id = "fetchtweets"
  arn       = "${aws_lambda_function.twitter_to_email.arn}"
}
resource "aws_lambda_permission" "allow_cloudwatch_to_call_twitter_to_email" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = "${aws_lambda_function.twitter_to_email.function_name}"
  principal     = "events.amazonaws.com"
  source_arn    = "${aws_cloudwatch_event_rule.every_five_minutes.arn}"
}


