package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/viswals_task/core/models"
	"github.com/viswals_task/pkg/database"
	"go.uber.org/zap"
	"io"
	"net/http"
	"strconv"
	"time"
)

var (
	defaultTimeout = 5 * time.Second
)

type UserService interface {
	GetAllUsers(context.Context) ([]*models.UserDetails, error)
	GetUser(context.Context, string) (*models.UserDetails, error)
	CreateUser(context.Context, *models.UserDetails) error
	DeleteUser(context.Context, string) error
	GetAllUsersSSE(ctx context.Context, limit, lastKey int64) ([]byte, error)
}

type Controller struct {
	UserService UserService
	logger      *zap.Logger
}

func New(userService UserService, logger *zap.Logger) *Controller {
	return &Controller{
		UserService: userService,
		logger:      logger,
	}
}

type httpResponse struct {
	StatusCode int         `json:"status_code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data"`
}

func (c *Controller) sendResponse(res http.ResponseWriter, statusCode int, message string, data interface{}) {
	res.Header().Add("Content-Type", "application/json")
	res.WriteHeader(statusCode)

	if statusCode == http.StatusNoContent {
		return
	}

	output := &httpResponse{
		StatusCode: statusCode,
		Message:    message,
		Data:       data,
	}
	err := json.NewEncoder(res).Encode(output)
	if err != nil {
		c.logger.Error("failed to marshal json", zap.Error(err), zap.String("response", message))
		res.WriteHeader(500)
		_, err := res.Write([]byte("{\"message\":\"internal server error\"}"))
		if err != nil {
			c.logger.Error("failed to write response", zap.Error(err), zap.String("response", message))
			return
		}
		return
	}
}

func (c *Controller) GetAllUsers(res http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), defaultTimeout)
	defer cancel()

	users, err := c.UserService.GetAllUsers(ctx)
	if err != nil {
		c.sendResponse(res, http.StatusInternalServerError, "failed to get all users", nil)
		return
	}

	c.sendResponse(res, http.StatusOK, "all users", users)
}

func (c *Controller) GetUser(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		c.sendResponse(res, http.StatusBadRequest, "user id is not provided in req or empty id, please check url. it should be /users/:id", nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	user, err := c.UserService.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			c.sendResponse(res, http.StatusRequestTimeout, "deadline exceed please try again after some time.", nil)
			return
		}
		if errors.Is(err, database.ErrNoData) {
			c.sendResponse(res, http.StatusNotFound, "requested data not found", nil)
			return
		}
		c.logger.Error("failed to get user", zap.Error(err), zap.String("id", id))
		c.sendResponse(res, http.StatusInternalServerError, "internal server error", nil)
		return
	}

	c.sendResponse(res, http.StatusOK, "success", user)
}

func (c *Controller) CreateUser(res http.ResponseWriter, req *http.Request) {

	body, err := io.ReadAll(req.Body)
	if err != nil {
		c.sendResponse(res, http.StatusInternalServerError, "failed to read request body", nil)
		return
	}

	defer req.Body.Close()

	var user models.UserDetails

	err = json.Unmarshal(body, &user)
	if err != nil {
		c.sendResponse(res, http.StatusInternalServerError, "failed to unmarshal request body", nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	err = c.UserService.CreateUser(ctx, &user)
	if err != nil {
		if errors.Is(err, database.ErrDuplicate) {
			c.sendResponse(res, http.StatusConflict, "user already exist in database", nil)
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			c.sendResponse(res, http.StatusRequestTimeout, "request time out please try again later", nil)
			return
		}

		c.sendResponse(res, http.StatusInternalServerError, "internal server error can't fetch data from database", nil)
		return
	}

	c.sendResponse(res, http.StatusCreated, "data created successfully", nil)
}

func (c *Controller) DeleteUser(res http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		c.sendResponse(res, http.StatusBadRequest, "request does not contains any id for user", nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	err := c.UserService.DeleteUser(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNoData) {
			c.sendResponse(res, http.StatusNoContent, "requested data not found or already deleted", nil)
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			c.sendResponse(res, http.StatusRequestTimeout, "request time out please try again later", nil)
			return
		}
		c.sendResponse(res, http.StatusInternalServerError, "internal server error can't fetch data from database", nil)
		return
	}

	c.sendResponse(res, http.StatusNoContent, "success", nil)

}

func (c *Controller) GetAllUsersSSE(res http.ResponseWriter, req *http.Request) {
	// default value
	var limit int64 = 10
	var offset int64 = 1
	var err error
	q := req.URL.Query()

	if q.Get("limit") != "" {
		limit, err = strconv.ParseInt(q.Get("limit"), 10, 64)
		if err != nil {
			c.sendResponse(res, http.StatusBadRequest, "failed to parse limit parameter", nil)
			return
		}
	}

	// Set essential headers
	res.Header().Set("Content-Type", "text/event-stream")
	res.Header().Set("Cache-Control", "no-cache")

	res.Header().Set("Access-Control-Allow-Origin", "*")

	// flusher to send data immediately to the client using Flush function
	flusher, ok := res.(http.Flusher)
	if !ok {
		c.sendResponse(res, http.StatusInternalServerError, "internal server error", nil)
		return
	}
	var isLastData bool
	for {

		data, err := c.UserService.GetAllUsersSSE(context.Background(), limit, offset*limit)
		if err != nil {
			if errors.Is(err, database.ErrNoData) {
				isLastData = true
			}
		}
		_, err = fmt.Fprint(res, "data: "+string(data)+"\n\n")
		if err != nil {
			c.logger.Error("failed to write response", zap.Error(err))
			c.sendResponse(res, http.StatusInternalServerError, "fail to send data to client", nil)
			return
		}
		flusher.Flush()
		time.Sleep(2 * time.Second)

		if isLastData {
			break
		}

		// increasing offset for next batch
		offset++
	}

	_, err = fmt.Fprint(res, "data: END\n\n")
	if err != nil {
		c.logger.Error("failed to write response", zap.Error(err))
		c.sendResponse(res, http.StatusInternalServerError, "fail to send data to client", nil)
		return
	}
	flusher.Flush()
}
