package fwdpub

import (
	"fmt"
	"strings"
)

type Publisher struct {
	Output        bool
	PublisherName string
	ProducerName  string
}

func (p *Publisher) MakeProducer(producer string) Publisher {
	p.ProducerName = producer
	return *p
}

func (p *Publisher) Write(b []byte) (int, error) {
	readLine := string(b)
	strings.TrimSuffix(readLine, "\n")

	if p.Output {
		fmt.Printf("%s, %s, %s", p.PublisherName, p.ProducerName, readLine)
	}
	return 0, nil
}
