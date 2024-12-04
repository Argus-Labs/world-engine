package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"
)

type ErrorResponse struct {
	Error Error `json:"error"`
}

type Error struct {
	Message string `json:"message"`
}

var ErrorHandler = func(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	c.Set(fiber.HeaderContentType, "application/json")

	return c.Status(code).JSON(ErrorResponse{Error: Error{Message: err.Error()}})
}
