package handler

import "github.com/gofiber/fiber/v2"

func responseInternalError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"message": err.Error(),
	})
}
