package tenant

import "context"

// contextKey is an unexported type for context keys in this package.
type contextKey struct{}

// SetID stores the tenant ID in the context.
func SetID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// GetID retrieves the tenant ID from the context.
// Returns the ID and true if found, or empty string and false if not.
func GetID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKey{}).(string)
	return id, ok
}
