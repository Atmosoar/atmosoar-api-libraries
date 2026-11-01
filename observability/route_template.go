package observability

import "context"

// routeTemplateKey is the request-context key WithRouteTemplate stores under.
// An unexported struct type, so no other package can collide with it.
type routeTemplateKey struct{}

// WithRouteTemplate returns a copy of ctx carrying the route template the
// request matched. It is for handlers that route requests themselves instead
// of through Gin's router (the gateway's NoRoute dispatcher), where
// gin.Context.FullPath is empty: HTTPMetricsMiddleware then labels the request
// with this template instead of "/unknown".
//
// The template becomes a Prometheus label value, so it must come from a fixed
// set such as a route table ("/forecast/v1", "/observations/stations/:id"),
// never from the raw request path. An empty template is ignored.
//
// Opt-in: services that never call it keep labelling unmatched requests
// "/unknown", and a request Gin matched keeps its FullPath either way.
func WithRouteTemplate(ctx context.Context, template string) context.Context {
	if template == "" {
		return ctx
	}
	return context.WithValue(ctx, routeTemplateKey{}, template)
}

// RouteTemplate returns the route template stored by WithRouteTemplate, and
// false when there is none.
func RouteTemplate(ctx context.Context) (string, bool) {
	template, ok := ctx.Value(routeTemplateKey{}).(string)
	return template, ok && template != ""
}
