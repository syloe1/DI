package mq

import (
	"encoding/json"
	"log"
	"sync"

	"go-admin/internal/gateway"

	"github.com/streadway/amqp"
)

type GroupMessageEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	GroupID   string `json:"group_id"`
	FromUID   string `json:"from_uid"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type ConsumerPool struct {
	conn           *amqp.Connection
	channel        *amqp.Channel
	queueName      string
	workerNum      int
	tasks          chan amqp.Delivery
	quitChan       chan struct{}
	sessionManager gateway.SessionManager
}

func NewConsumerPool(conn *amqp.Connection, queueName string, workerNum int, sm gateway.SessionManager) *ConsumerPool {
	if workerNum <= 0 {
		workerNum = 1
	}

	return &ConsumerPool{
		conn:           conn,
		queueName:      queueName,
		workerNum:      workerNum,
		tasks:          make(chan amqp.Delivery, workerNum*2),
		quitChan:       make(chan struct{}),
		sessionManager: sm,
	}
}

func (p *ConsumerPool) Run() error {
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	p.channel = ch

	if err := ch.Qos(p.workerNum*2, 0, false); err != nil {
		_ = ch.Close()
		return err
	}

	deliveries, err := ch.Consume(
		p.queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		return err
	}

	var wg sync.WaitGroup
	for i := 0; i < p.workerNum; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.workerLoop()
		}()
	}

	go func() {
		defer close(p.tasks)
		for {
			select {
			case <-p.quitChan:
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}
				select {
				case <-p.quitChan:
					return
				case p.tasks <- d:
				}
			}
		}
	}()

	<-p.quitChan
	wg.Wait()
	return ch.Close()
}

func (p *ConsumerPool) Stop() {
	select {
	case <-p.quitChan:
	default:
		close(p.quitChan)
	}
}

func (p *ConsumerPool) workerLoop() {
	for d := range p.tasks {
		var event GroupMessageEvent
		if err := json.Unmarshal(d.Body, &event); err != nil {
			log.Printf("unmarshal group message event failed: %v", err)
			_ = d.Nack(false, false)
			continue
		}

		p.pushToLocalUsers(event)
		_ = d.Ack(false)
	}
}

func (p *ConsumerPool) pushToLocalUsers(event GroupMessageEvent) {
	msgBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("marshal group message event failed: %v", err)
		return
	}

	for _, uid := range p.sessionManager.LocalUserIDs() {
		if ok := p.sessionManager.Enqueue(uid, msgBytes); !ok {
			log.Printf("send queue full or offline, uid=%s group=%s", uid, event.GroupID)
		}
	}
}
