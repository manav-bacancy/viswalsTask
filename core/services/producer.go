package services

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"github.com/viswals_task/core/models"
	"github.com/viswals_task/internal/csvutils"
	"go.uber.org/zap"
	"io"
	"strconv"
	"time"
)

var (
	defaultPublishTimeout = 15 * time.Second
)

type Producer struct {
	csvReader *csv.Reader
	queue     queuePublisher
	logger    *zap.Logger
}

func NewProducer(csvReader *csv.Reader, queue queuePublisher, logger *zap.Logger) *Producer {
	return &Producer{
		csvReader: csvReader,
		queue:     queue,
		logger:    logger,
	}
}

func (p *Producer) Start(batchSize int) error {
	// read batchSize data from csv reader
	var isLastRecord = false
	for {
		rows, err := csvutils.ReadRows(p.csvReader, batchSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				isLastRecord = true
				break
			} else {
				return err
			}
		}

		// transform fetched rows to user struct
		data := p.CsvToStruct(rows)
		if data == nil {
			continue
		}

		// publish data to queue
		ctx, cancel := context.WithTimeout(context.Background(), defaultPublishTimeout)
		err = p.Publish(ctx, data)
		if err != nil {
			cancel()
			return err
		}
		cancel()

		if isLastRecord {
			break
		}
	}

	return nil
}

func (p *Producer) Publish(ctx context.Context, data []*models.UserDetails) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if err := p.queue.Publish(ctx, jsonData); err != nil {
		return err
	}
	return nil
}

func (p *Producer) CsvToStruct(data [][]string) []*models.UserDetails {
	var result []*models.UserDetails

	for _, row := range data {
		userDetails := new(models.UserDetails)

		if len(row) != 8 {
			continue
		}

		id, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			p.logger.Warn("failed to convert string to int", zap.Error(err))
			continue
		}
		userDetails.ID = id

		userDetails.FirstName = row[1]

		userDetails.LastName = row[2]

		userDetails.EmailAddress = row[3]

		miliSec, err := strconv.ParseInt(row[4], 10, 64)
		if err != nil {
			// ignoring the error for now
			p.logger.Warn("failed to convert string to int", zap.Error(err))
			continue
		} else {
			if miliSec == -1 {
				userDetails.CreatedAt = sql.NullTime{
					Valid: false,
				}
			} else {
				userDetails.CreatedAt = sql.NullTime{
					Time:  time.UnixMilli(miliSec),
					Valid: true,
				}
			}
		}

		miliSec, err = strconv.ParseInt(row[5], 10, 64)
		if err != nil {
			// ignoring the error for now
			p.logger.Warn("failed to convert string to int", zap.Error(err))
			continue
		} else {
			if miliSec == -1 {
				userDetails.DeletedAt = sql.NullTime{
					Valid: false,
				}
			} else {
				userDetails.DeletedAt = sql.NullTime{
					Time:  time.UnixMilli(miliSec),
					Valid: true,
				}
			}
		}

		miliSec, err = strconv.ParseInt(row[6], 10, 64)
		if err != nil {
			// ignoring the error for now
			p.logger.Warn("failed to convert string to int", zap.Error(err))
			continue
		} else {
			if miliSec == -1 {
				userDetails.MergedAt = sql.NullTime{
					Valid: false,
				}
			} else {
				userDetails.MergedAt = sql.NullTime{
					Time:  time.UnixMilli(miliSec),
					Valid: true,
				}
			}
		}

		parentUserId, err := strconv.ParseInt(row[7], 10, 64)
		if err != nil {
			p.logger.Warn("failed to convert string to int", zap.Error(err))
			continue
		}

		userDetails.ParentUserId = parentUserId

		result = append(result, userDetails)
	}

	return result
}

func (p *Producer) Close() error {
	return p.queue.Close()
}
