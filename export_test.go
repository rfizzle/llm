package llm

// What the package's external tests reach that no app should: the gateway
// path's body rewrite, and the classifier's table and bound.
var (
	MarkOpenAICacheBody = markOpenAICacheBody
	Sentinels           = sentinels
)

const MaxMessage = maxMessage
