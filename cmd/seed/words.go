package main

import "math/rand/v2"

// Three short lists make a thousand posts that read like people wrote them.
var (
	openers = []string{"Finally,", "Today", "Quick thought:", "Honestly,",
		"Small win:", "Note to self:", "This morning", "Somehow",
		"Fair warning:", "Plot twist:"}
	subjects = []string{"the coffee here is", "my sourdough is", "the train is",
		"this book is", "the new build is", "the garden is", "the rain is",
		"the cat is", "the playlist is", "a quiet Sunday is"}
	endings = []string{"exactly right.", "better than expected.",
		"still a mystery.", "worth the wait.", "a little too loud.",
		"good enough for now.", "the best part of the day.",
		"doing its own thing.", "somehow perfect.", "not what I ordered."}
)

// writer returns a function that writes one post after another. It has a
// generator of its own, so the graph and who posts when are what they were.
func writer(seed uint64) func() string {
	r := rand.New(rand.NewPCG(seed, 1))
	pick := func(words []string) string { return words[r.IntN(len(words))] }
	return func() string {
		return pick(openers) + " " + pick(subjects) + " " + pick(endings)
	}
}
