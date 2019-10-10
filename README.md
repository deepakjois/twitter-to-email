# twitter-to-email
Get tweets from your Twitter Home timeline as a daily email digest.

## Description and Usage
See Post on Medium: [Getting your Twitter Timeline as an Email Digest using AWS](https://medium.com/@debugjois/getting-your-twitter-timeline-as-an-email-digest-using-aws-e1c168589734)

## Quickstart
* Install [awscli], [Go] and [Terraform].
* Create a bucket to store tweets: `aws s3 mb s3://<bucket_to_store_tweets>`
* Configure Lambda function using a JSON file: `cp config.json.example config.json`
* Build Lambda Package: `./build-lambda-package.sh`
* Change all instances of `twitter-to-email-debugjois` in `deploy.tf` to the bucket name you created above.
* Run `terraform plan`, and then `terraform apply`

[awscli]: https://aws.amazon.com/cli/
[Go]: https://golang.org
[Terraform]: https://terraform.io

