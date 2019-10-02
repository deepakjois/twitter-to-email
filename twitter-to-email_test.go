package main

import "testing"

func TestFetchTweets(t *testing.T) {
	getConfig()
	err := fetchTweets()
	if err != nil {
		t.Errorf("There was a problem: %v", err)
	}
}
