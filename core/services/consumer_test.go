package services

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/viswals_task/core/models"
	"github.com/viswals_task/internal/encryptionutils"
	"github.com/viswals_task/pkg/database/mockdatabase"
	"github.com/viswals_task/pkg/rabbitmq/mockrabbitmq"
	"github.com/viswals_task/pkg/redis/mockredis"
	"go.uber.org/zap"
)

// test the consume work flow with valid inputs
type TestCases struct {
	name     string
	consumer *Consumer
	channel  chan amqp.Delivery
	body     []byte
}

func TestConsumer(t *testing.T) {
	userStore := new(mockdatabase.MockDatabase)
	memStore := new(mockredis.MockRedis)
	queueStore := new(mockrabbitmq.MockRabbitMQ)

	log, err := zap.NewDevelopment()
	assert.NoError(t, err)

	encryp, err := encryptionutils.New([]byte("testtesttesttest"))
	assert.NoError(t, err)

	userStore.On("CreateUser", mock.Anything, mock.AnythingOfType("*models.UserDetails")).Return(nil)
	memStore.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("*models.UserDetails")).Return(nil)

	var deliveryChannel = make(chan amqp.Delivery, 10)

	tt := []TestCases{
		{
			name: "Valid Input",
			consumer: &Consumer{
				queue:     queueStore,
				channel:   deliveryChannel,
				encryp:    encryp,
				logger:    log,
				userStore: userStore,
				memStore:  memStore,
			},
			channel: deliveryChannel,
			body: []byte(`
			[
				{
					"id":1,
					"first_name":"John",
					"last_name":"Doe",
					"email_address":"john@doe.com",
					"created_at":null,
					"deleted_at":null,
					"merged_at":null,
					"parent_user_id":1
				}
			]
		`),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var wg = new(sync.WaitGroup)
			wg.Add(1)

			go tc.consumer.Consume(wg, 1)

			tc.channel <- amqp.Delivery{
				Body: tc.body,
			}

			time.Sleep(2 * time.Second)
			close(tc.channel)

			wg.Wait()
		})
	}

	userStore.AssertExpectations(t)
	memStore.AssertExpectations(t)
	queueStore.AssertExpectations(t)
}

type TestUserDetails struct {
	name       string
	input      []byte
	output     []*models.UserDetails
	throwError bool
}

func TestToUserDetails(t *testing.T) {
	log, err := zap.NewDevelopment()
	assert.NoError(t, err)

	encryp, err := encryptionutils.New([]byte("testtesttesttest"))
	assert.NoError(t, err)

	var testCases = []TestUserDetails{
		{
			name: "valid input",
			input: []byte(`
			[
				{
					"id":1,
					"first_name":"John",
					"last_name":"Doe",
					"email_address":"john@doe.com",
					"created_at":null,
					"deleted_at":null,
					"merged_at":null,
					"parent_user_id":1
				}
			]
		`),
			output: []*models.UserDetails{{
				ID:           1,
				FirstName:    "John",
				LastName:     "Doe",
				EmailAddress: "john@doe.com",
				CreatedAt:    sql.NullTime{},
				DeletedAt:    sql.NullTime{},
				MergedAt:     sql.NullTime{},
				ParentUserId: 1,
			}},
			throwError: false,
		},
		{
			name:       "nil input",
			input:      nil,
			output:     nil,
			throwError: true,
		},
		{
			name: "wrong type input",
			input: []byte(`
			[
				{
					"id":1,
					"first_name": 546,
					"last_name":"Doe",
					"email_address":"john@doe.com"
					"created_at":null
					"deleted_at":null
					"merged_at":null
					"parent_user_id":1
				}
			]
		`),
			output:     nil,
			throwError: true,
		},
	}

	// Create a blank consumer first
	consumer := Consumer{
		encryp: encryp,
		logger: log,
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			var inputChan = make(chan []byte, 10)
			var outputChan = make(chan []*models.UserDetails, 10)
			var errorChan = make(chan error)
			wg := new(sync.WaitGroup)
			wg.Add(1)
			go consumer.ToUserDetails(wg, inputChan, outputChan, errorChan)
			inputChan <- tt.input
			close(inputChan)
			if tt.throwError {
				e, ok := <-errorChan
				if !ok {
					e = nil
				}
				assert.Error(t, e)
			}
			o, ok := <-outputChan
			if !ok {
				o = nil
			}
			assert.Equal(t, tt.output, o)
			wg.Wait()
		})
	}
}

type TestSaveDetails struct {
	name       string
	input      []*models.UserDetails
	consumer   *Consumer
	throwError bool
}

func TestSaveUserDetails(t *testing.T) {
	mockUserStore := new(mockdatabase.MockDatabase)
	mockUserStoreWithError := new(mockdatabase.MockDatabase)

	encryp, err := encryptionutils.New([]byte("testtesttesttest"))
	assert.NoError(t, err)

	mockMemStore := new(mockredis.MockRedis)

	mockUserStore.On("CreateUser", mock.Anything, mock.AnythingOfType("*models.UserDetails")).Return(nil)
	mockUserStoreWithError.On("CreateUser", mock.Anything, mock.AnythingOfType("*models.UserDetails")).Return(errors.New("test error"))

	mockMemStore.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("*models.UserDetails")).Return(nil)

	log, err := zap.NewDevelopment()
	assert.NoError(t, err)

	testCases := []TestSaveDetails{
		{
			name: "valid input",
			input: []*models.UserDetails{
				{FirstName: "John",
					LastName:     "Doe",
					EmailAddress: "john@doe.com",
					CreatedAt: sql.NullTime{
						Time:  time.Now(),
						Valid: true,
					},
					DeletedAt: sql.NullTime{
						Time:  time.Now(),
						Valid: true,
					},
					MergedAt: sql.NullTime{
						Time:  time.Now(),
						Valid: true,
					},
					ParentUserId: 1},
			},
			consumer: &Consumer{
				queue:     nil,
				channel:   nil,
				logger:    log,
				encryp:    encryp,
				userStore: mockUserStore,
				memStore:  mockMemStore,
			},
			throwError: false,
		},
		{
			name: "database error",
			input: []*models.UserDetails{
				{FirstName: "John",
					LastName:     "Doe",
					EmailAddress: "john@doe.com",
					CreatedAt:    sql.NullTime{},
					DeletedAt:    sql.NullTime{},
					MergedAt:     sql.NullTime{},
					ParentUserId: 1},
			},
			consumer: &Consumer{
				queue:     nil,
				channel:   nil,
				logger:    log,
				encryp:    encryp,
				userStore: mockUserStoreWithError,
				memStore:  mockMemStore,
			},
			throwError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var inputChan = make(chan []*models.UserDetails, 10)
			var errorChan = make(chan error, 10)
			wg := new(sync.WaitGroup)
			wg.Add(1)
			go testCase.consumer.SaveUserDetails(wg, inputChan, errorChan)
			inputChan <- testCase.input
			close(inputChan)
			if testCase.throwError {
				assert.Error(t, <-errorChan)
			}
			wg.Wait()
		})
	}

	mockUserStore.AssertExpectations(t)
	mockMemStore.AssertExpectations(t)
	mockUserStoreWithError.AssertExpectations(t)
}

func TestCloseConsumer(t *testing.T) {
	mockQueue := new(mockrabbitmq.MockRabbitMQ)
	consumer := &Consumer{
		queue: mockQueue,
	}

	mockQueue.On("Close").Return(nil)

	err := consumer.Close()
	assert.NoError(t, err)
}
