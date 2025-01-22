package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/viswals_task/core/models"
	"github.com/viswals_task/internal/encryptionutils"
	"github.com/viswals_task/pkg/database"
	"go.uber.org/zap"
	"sync"
	"time"
)

var (
	defaultTimeout = time.Second * 15
)

type Consumer struct {
	queue   queueConsumer
	channel <-chan amqp.Delivery
	logger  *zap.Logger
	// extending consumer to provide http server and database access.
	userStore dataStoreProvider
	memStore  memoryStoreProvider
}

func NewConsumer(queue queueConsumer, userStore dataStoreProvider, memStore memoryStoreProvider, logger *zap.Logger) (*Consumer, error) {
	// connect with the initialized queue.
	in, err := queue.Subscribe()
	if err != nil {
		return nil, err
	}

	return &Consumer{
		queue:     queue,
		channel:   in,
		logger:    logger,
		userStore: userStore,
		memStore:  memStore,
	}, nil
}

func (c *Consumer) Consume(wg *sync.WaitGroup, size int) {
	defer wg.Done()

	var userDetailsInput chan []byte = make(chan []byte, size)
	var userDetailsOutput chan []*models.UserDetails = make(chan []*models.UserDetails, size)

	var errorChan chan error = make(chan error, 10)

	var internalWg = new(sync.WaitGroup)

	internalWg.Add(1)
	go c.ToUserDetails(internalWg, userDetailsInput, userDetailsOutput, errorChan)

	internalWg.Add(1)
	go c.SaveUserDetails(internalWg, userDetailsOutput, errorChan)

	internalWg.Add(1)
	go c.errorLogger(internalWg, errorChan)

	for data := range c.channel {
		body := data.Body

		if body == nil {
			continue
		}

		userDetailsInput <- body
		//details, err := c.ToUserDetails(body)
		//if err != nil {
		//	c.logger.Error("error deserializing user details", zap.Error(err), zap.String("body", string(body)))
		//	continue
		//}

		//dataToInsert := make([]*models.UserDetails, 0, len(details))

		//for _, user := range details {
		// encrypt Email id for user

		//encryptedEmail, err := encryptionutils.Encrypt(user.EmailAddress)
		//if err != nil {
		//	c.logger.Error("error encrypting email", zap.Error(err), zap.String("email", user.EmailAddress))
		//	continue
		//}
		//user.EmailAddress = encryptedEmail
		//
		//ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)

		// store user detail in the storage.
		//err = c.SaveUserDetails(ctx, user)
		//if err != nil {
		//	cancel()
		//	c.logger.Error("failed to save user details", zap.Error(err))
		//	continue
		//}

		//cancel()
		//}

		// upon successful acknowledge the data.
		//err = data.Ack(false)
		//if err != nil {
		//	c.logger.Error("failed to ack user details", zap.Error(err), zap.Any("details", details))
		//}

	}

	c.logger.Info(fmt.Sprintf("Consumer stopped"))
	close(userDetailsInput)
	internalWg.Wait()
}

func (c *Consumer) errorLogger(wg *sync.WaitGroup, errorChan chan error) {
	defer wg.Done()
	for e := range errorChan {
		if e == nil {
			c.logger.Error("error in consumer as", zap.Error(e))
		}
	}
}

func (c *Consumer) ToUserDetails(wg *sync.WaitGroup, inputChan chan []byte, outputChan chan []*models.UserDetails, errorChan chan error) {
	defer wg.Done()
	defer close(outputChan)
	for data := range inputChan {
		var users []*models.UserDetails
		err := json.Unmarshal(data, &users)
		if err != nil {
			errorChan <- err
			continue
		}
		outputChan <- users
	}
	c.logger.Info(fmt.Sprintf("User Data Marsheller stopped"))
	//outputChan <- nil
}

//func (c *Consumer) EncryptEmail(wg *sync.WaitGroup, inputChan chan []*models.UserDetails, outputChan chan []*models.UserDetails, errorChan chan error) {
//	defer wg.Done()
//	defer close(outputChan)
//	for data := range inputChan {
//		for _, u := range data {
//			encryptedEmail, err := encryptionutils.Encrypt(u.EmailAddress)
//			if err != nil {
//				errorChan <- err
//				continue
//			}
//			u.EmailAddress = encryptedEmail
//		}
//		outputChan <- data
//	}
//	c.logger.Info(fmt.Sprintf("User Data Encrypter stopped"))
//}

func (c *Consumer) SaveUserDetails(wg *sync.WaitGroup, inputChan chan []*models.UserDetails, errorChan chan error) {
	defer wg.Done()
	defer close(errorChan)
	for data := range inputChan {
		for _, user := range data {

			encryptedEmail, err := encryptionutils.Encrypt(user.EmailAddress)
			if err != nil {
				c.logger.Error("error encrypting user", zap.Error(err))
				errorChan <- err
				continue
			}
			user.EmailAddress = encryptedEmail

			ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
			// first save user details to a database.
			err = c.userStore.CreateUser(ctx, user)
			if err != nil {
				if errors.Is(err, database.ErrDuplicate) {
					c.logger.Warn("User already exists", zap.Error(err), zap.Any("user", user))
				}

				errorChan <- err
				cancel()
				continue
			}

			// second save to use details to inMemoryDatabase.
			err = c.memStore.Set(ctx, fmt.Sprint(user.ID), user)
			if err != nil {
				// not to worry as data has been already stored in a database.
				c.logger.Warn("Failed to store user in memoryDatabase", zap.Error(err), zap.Any("user", user))
			}
			cancel()
		}
	}
	c.logger.Info(fmt.Sprintf("User Data Saver stopped"))
}

func (c *Consumer) Close() error {
	return c.queue.Close()
}
