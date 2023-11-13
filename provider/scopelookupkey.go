package provider

import (
	"github.com/lyraproj/dgo/dgo"
	"github.com/lyraproj/hierasdk/hiera"
	"github.com/yanndegat/hiera/api"
)

// ScopeLookupKey is a function that performs a lookup in the current scope.
func ScopeLookupKey(pc hiera.ProviderContext, key string) dgo.Value {
	sc, ok := pc.(api.ServerContext)
	if !ok {
		return nil
	}
	return sc.Invocation().Scope().Get(key)
}
