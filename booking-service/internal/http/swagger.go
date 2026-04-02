package http

import "github.com/gofiber/fiber/v2"

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Booking Service API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/docs/openapi.yaml',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis],
    });
  </script>
</body>
</html>`

func registerSwaggerRoutes(app *fiber.App) {
	app.Static("/docs", "./docs")
	app.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Type("html").SendString(swaggerHTML)
	})
	app.Get("/swagger/", func(c *fiber.Ctx) error {
		return c.Type("html").SendString(swaggerHTML)
	})
}
