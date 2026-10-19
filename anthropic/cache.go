package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfizzle/llm/cache"
	"github.com/rfizzle/llm/internal/cachemarks"
)

// markAnthropicCache places the plan's markers on a Messages API request, all
// of them at the session's lifetime (cache.DefaultTTL).
func markAnthropicCache(params *anthropic.MessageNewParams, ttl cache.TTL) {
	ttl = cache.TTLOrDefault(string(ttl))
	marks := cachemarks.Plan(len(params.System) > 0, len(params.Messages))
	if marks.Head {
		params.System[len(params.System)-1].CacheControl = cacheControl(ttl)
	}
	for _, i := range marks.Messages {
		markLastBlock(params.Messages[i].Content, ttl)
	}
}

// cacheControl is one marker at a chosen lifetime.
func cacheControl(ttl cache.TTL) anthropic.CacheControlEphemeralParam {
	cc := anthropic.NewCacheControlEphemeralParam()
	cc.TTL = anthropic.CacheControlEphemeralTTL(ttl)
	return cc
}

// markLastBlock marks the last block of a message that can carry a marker,
// and reports whether it found one.
//
// It searches from the end rather than taking the final block outright
// because not every block type has the field — a turn that is nothing but
// reasoning ends on a block the API will not accept a marker on, and a marker
// placed there would be a request rejected for the sake of a saving. Marking
// an earlier block of the same message is worth doing anyway: the prefix it
// caches is shorter by the tail of one message and still covers everything
// before it.
func markLastBlock(blocks []anthropic.ContentBlockParamUnion, ttl cache.TTL) bool {
	for i := len(blocks) - 1; i >= 0; i-- {
		if cc := blocks[i].GetCacheControl(); cc != nil {
			*cc = cacheControl(ttl)
			return true
		}
	}
	return false
}
