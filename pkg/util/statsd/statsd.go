package statsd

import (
	stats "gopkg.in/alexcesaro/statsd.v2"
)

var client *stats.Client

func Initialize(address, prefix string) error {
	var err error
	client, err = stats.New(stats.Address(address), stats.Prefix(prefix))
	return err
}

func Count(bucket string, n interface{}) {
	if client != nil {
		client.Count(bucket, n)
	}
}

func Gauge(bucket string, value interface{}) {
	if client != nil {
		client.Gauge(bucket, value)
	}
}

func Increment(bucket string) {
	if client != nil {
		client.Increment(bucket)
	}
}

func Timing(bucket string, value interface{}) {
	if client != nil {
		client.Timing(bucket, value)
	}
}
