package fwdpub

import (
	"testing"
)

func TestPublisher_Write_OutputDisabled(t *testing.T) {
	p := &Publisher{
		PublisherName: "test-pub",
		Output:        false,
	}

	n, err := p.Write([]byte("test message"))
	if err != nil {
		t.Errorf("Write returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written, got %d", n)
	}
}

func TestPublisher_Write_OutputEnabled(t *testing.T) {
	p := &Publisher{
		PublisherName: "test-pub",
		ProducerName:  "test-producer",
		Output:        true,
	}

	// This will print to stdout, but should not error
	n, err := p.Write([]byte("test message"))
	if err != nil {
		t.Errorf("Write with output enabled returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written, got %d", n)
	}
}

func TestPublisher_MakeProducer(t *testing.T) {
	p := &Publisher{
		PublisherName: "services",
		Output:        false,
	}

	producer := p.MakeProducer("service.default:8080")

	if producer.ProducerName != "service.default:8080" {
		t.Errorf("Expected producer name 'service.default:8080', got %s", producer.ProducerName)
	}

	if producer.PublisherName != "services" {
		t.Errorf("Producer should inherit PublisherName, got %s", producer.PublisherName)
	}

	if producer.Output != false {
		t.Error("Producer should inherit Output setting")
	}
}

func TestPublisher_MakeProducer_ModifiesOriginal(t *testing.T) {
	p := &Publisher{
		PublisherName: "services",
		Output:        true,
	}

	_ = p.MakeProducer("producer1")

	// The original publisher is also modified
	if p.ProducerName != "producer1" {
		t.Errorf("MakeProducer should modify original, got %s", p.ProducerName)
	}
}

func TestPublisher_Write_StripsNewline(t *testing.T) {
	p := &Publisher{
		PublisherName: "test",
		Output:        false,
	}

	// Should not error with newline
	_, err := p.Write([]byte("message with newline\n"))
	if err != nil {
		t.Errorf("Write with newline returned error: %v", err)
	}

	// Should not error with multiple newlines
	_, err = p.Write([]byte("message\n\n\n"))
	if err != nil {
		t.Errorf("Write with multiple newlines returned error: %v", err)
	}
}

func TestPublisher_Write_EmptyMessage(t *testing.T) {
	p := &Publisher{
		PublisherName: "test",
		Output:        false,
	}

	n, err := p.Write([]byte(""))
	if err != nil {
		t.Errorf("Write with empty message returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written for empty message, got %d", n)
	}
}

func TestPublisher_Write_NilSlice(t *testing.T) {
	p := &Publisher{
		PublisherName: "test",
		Output:        false,
	}

	n, err := p.Write(nil)
	if err != nil {
		t.Errorf("Write with nil slice returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written for nil slice, got %d", n)
	}
}

func TestPublisher_ZeroValue(t *testing.T) {
	p := &Publisher{}

	n, err := p.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write on zero value Publisher returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written, got %d", n)
	}
}

func TestPublisher_ImplementsWriter(_ *testing.T) {
	var _ interface {
		Write([]byte) (int, error)
	} = &Publisher{}
}

func TestPublisher_MakeProducer_Multiple(t *testing.T) {
	p := &Publisher{
		PublisherName: "base",
		Output:        false,
	}

	// Create multiple producers
	prod1 := p.MakeProducer("producer1")
	prod2 := p.MakeProducer("producer2")
	prod3 := p.MakeProducer("producer3")

	// All producers share the same PublisherName
	if prod1.PublisherName != "base" || prod2.PublisherName != "base" || prod3.PublisherName != "base" {
		t.Error("All producers should share the same PublisherName")
	}

	// The original publisher has the last producer name
	if p.ProducerName != "producer3" {
		t.Errorf("Original should have last producer name, got %s", p.ProducerName)
	}
}

func TestPublisher_LongMessage(t *testing.T) {
	p := &Publisher{
		PublisherName: "test",
		Output:        false,
	}

	// Create a long message
	longMsg := make([]byte, 10000)
	for i := range longMsg {
		longMsg[i] = 'x'
	}

	n, err := p.Write(longMsg)
	if err != nil {
		t.Errorf("Write with long message returned error: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected 0 bytes written, got %d", n)
	}
}
