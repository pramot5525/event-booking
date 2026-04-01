package middleware

import "github.com/gofiber/fiber/v2"

// APIKeyAuth returns a middleware that validates the X-API-Key header.
// If apiKey is empty (not configured), the middleware is a no-op (development mode).
func APIKeyAuth(apiKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if apiKey == "" {
			return c.Next()
		}
		if c.Get("X-API-Key") != apiKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}
