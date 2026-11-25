# REST API

## Compression

The API compresses JSON responses with gzip when the `Accept-Encoding` request
header allows gzip with a nonzero quality value.
The API does not compress responses smaller than 1024 bytes.
The API does not compress responses of the IncusOS proxy endpoints `/os/` and
`/1.0/provisioning/servers/{name}/os`.
Responses outside the IncusOS proxy endpoints include `Vary: Accept-Encoding`.

## Reference

<link rel="stylesheet" type="text/css" href="../../_static/swagger-ui/swagger-ui.css" ></link>
<link rel="stylesheet" type="text/css" href="../../_static/swagger-override.css" ></link>
<div id="swagger-ui"></div>

<script src="../../_static/swagger-ui/swagger-ui-bundle.js" charset="UTF-8"> </script>
<script src="../../_static/swagger-ui/swagger-ui-standalone-preset.js" charset="UTF-8"> </script>
<script>
window.onload = function() {
  // Begin Swagger UI call region
  const ui = SwaggerUIBundle({
    url: window.location.pathname +"../../rest-api.yaml",
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [
      SwaggerUIBundle.presets.apis,
      SwaggerUIStandalonePreset
    ],
    plugins: [],
    validatorUrl: "none",
    defaultModelsExpandDepth: -1,
    supportedSubmitMethods: []
  })
  // End Swagger UI call region

  window.ui = ui
}
</script>
