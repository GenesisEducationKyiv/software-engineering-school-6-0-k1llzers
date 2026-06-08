package rules

type Rule[T any, R any] struct {
	Match  func(T) bool
	Handle func(T) R
}

type Matcher[T any, R any] struct {
	rules         []Rule[T, R]
	defaultHandle func(T) R
}

func NewMatcher[T any, R any](rules []Rule[T, R], defaultHandle func(T) R) Matcher[T, R] {
	return Matcher[T, R]{
		rules:         rules,
		defaultHandle: defaultHandle,
	}
}

func (m Matcher[T, R]) Resolve(input T) R {
	for _, rule := range m.rules {
		if rule.Match(input) {
			return rule.Handle(input)
		}
	}

	return m.defaultHandle(input)
}
