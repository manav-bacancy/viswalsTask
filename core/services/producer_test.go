package services

import (
	"context"
	"database/sql"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/viswals_task/core/models"
	"github.com/viswals_task/internal/csvutils"
	"github.com/viswals_task/pkg/rabbitmq/mockrabbitmq"
	"go.uber.org/zap"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	batchSize := 1
	mockQueue := new(mockrabbitmq.MockRabbitMQ)

	mockQueue.On("Publish", mock.Anything, mock.AnythingOfType("[]uint8")).Return(nil)

	csvReader, err := csvutils.OpenFile("../../csvfiles/test.csv")
	assert.NoError(t, err)

	// read the metadata fields
	//data, err := csvReader.Read()
	//assert.NoError(t, err)
	//assert.NotNil(t, data)

	log, err := zap.NewDevelopment()
	assert.NoError(t, err)

	producer := &Producer{
		logger:    log,
		queue:     mockQueue,
		csvReader: csvReader,
	}
	err = producer.Start(batchSize)
	assert.NoError(t, err)

	mockQueue.AssertExpectations(t)
}

type PublisherTest struct {
	name       string
	producer   *Producer
	input      []*models.UserDetails
	throwError bool
}

func TestPublish(t *testing.T) {
	publisher := new(mockrabbitmq.MockRabbitMQ)
	publisherWithError := new(mockrabbitmq.MockRabbitMQ)

	publisher.On("Publish", mock.Anything, mock.AnythingOfType("[]uint8")).Return(nil)
	publisherWithError.On("Publish", mock.Anything, mock.AnythingOfType("[]uint8")).Return(errors.New("mock error"))
	log, err := zap.NewDevelopment()
	assert.NoError(t, err)

	testCases := []PublisherTest{
		{
			name: "Valid Input",
			producer: &Producer{
				queue:  publisher,
				logger: log,
			},
			input: []*models.UserDetails{
				{
					ID:           0,
					FirstName:    "test",
					LastName:     "test",
					EmailAddress: "test",
					CreatedAt:    sql.NullTime{},
					DeletedAt:    sql.NullTime{},
					MergedAt:     sql.NullTime{},
					ParentUserId: 0,
				},
			},
			throwError: false,
		}, {
			name: "publisher error",
			producer: &Producer{
				queue:  publisherWithError,
				logger: log,
			},
			input: []*models.UserDetails{
				{
					ID:           0,
					FirstName:    "test",
					LastName:     "test",
					EmailAddress: "test",
					CreatedAt:    sql.NullTime{},
					DeletedAt:    sql.NullTime{},
					MergedAt:     sql.NullTime{},
					ParentUserId: 0,
				},
			},
			throwError: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.producer.Publish(context.Background(), tt.input)
			if tt.throwError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
	publisherWithError.AssertExpectations(t)
	publisher.AssertExpectations(t)
}

type CsvParserTest struct {
	name   string
	input  [][]string
	output []*models.UserDetails
}

func TestCsvParser(t *testing.T) {
	testCases := []CsvParserTest{
		{
			name: "Valid Input and TimeStamps",
			input: [][]string{
				{"1", "test", "test", "test@test.com", "1737481973", "1737481973", "1737481973", "-1"},
			},
			output: []*models.UserDetails{
				{
					ID:           1,
					FirstName:    "test",
					LastName:     "test",
					EmailAddress: "test@test.com",
					CreatedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					DeletedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					MergedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					ParentUserId: -1,
				},
			},
		},
		{
			name: "Valid Input and null Timestamps",
			input: [][]string{
				{"1", "test", "test", "test@test.com", "-1", "1737481973", "1737481973", "-1"},
				{"1", "test", "test", "test@test.com", "1737481973", "-1", "1737481973", "-1"},
				{"1", "test", "test", "test@test.com", "1737481973", "1737481973", "-1", "-1"},
			},
			output: []*models.UserDetails{
				{
					ID:           1,
					FirstName:    "test",
					LastName:     "test",
					EmailAddress: "test@test.com",
					CreatedAt:    sql.NullTime{},
					DeletedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					MergedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					ParentUserId: -1,
				}, {
					ID:           1,
					FirstName:    "test",
					LastName:     "test",
					EmailAddress: "test@test.com",
					CreatedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					DeletedAt: sql.NullTime{},
					MergedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					ParentUserId: -1,
				}, {
					ID:           1,
					FirstName:    "test",
					LastName:     "test",
					EmailAddress: "test@test.com",
					CreatedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					DeletedAt: sql.NullTime{
						Time:  time.UnixMilli(1737481973),
						Valid: true,
					},
					MergedAt:     sql.NullTime{},
					ParentUserId: -1,
				},
			},
		},
		{
			name: "Invalid Inputs types",
			input: [][]string{
				{"not a number", "test", "test", "test@test.com", "-1", "-1", "-1", "-1"},
				{"1", "test", "test", "test@test.com", "not a timestamp", "-1", "-1", "-1"},
				{"1", "test", "test", "test@test.com", "-1", "not a timestamp", "-1", "-1"},
				{"1", "test", "test", "test@test.com", "-1", "-1", "not a timestamp", "-1"},
				{"1", "test", "test", "test@test.com", "-1", "-1", "-1", "not a valid number"},
			},
			output: nil,
		}, {
			name: "Missing Values",
			input: [][]string{
				{"first", "last", "test@test.com", "-1", "-1", "-1", "-1"},
				{"1", "last", "test@test.com", "-1", "-1", "-1", "-1"},
				{"1", "first", "last", "-1", "-1", "-1", "-1"},
				{"1", "first", "last", "test@test.com", "-1", "-1", "-1"},
			},
			output: nil,
		},
	}

	log, err := zap.NewDevelopment()
	assert.NoError(t, err)

	producer := &Producer{
		logger: log,
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			output := producer.CsvToStruct(tt.input)
			assert.Equal(t, tt.output, output)
		})
	}
}

func TestClose(t *testing.T) {
	publisher := new(mockrabbitmq.MockRabbitMQ)
	publisher.On("Close").Return(nil)

	producer := &Producer{
		queue: publisher,
	}

	err := producer.Close()

	assert.NoError(t, err)

	publisher.AssertExpectations(t)
}
