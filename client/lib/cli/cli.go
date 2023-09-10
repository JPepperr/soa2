package cli

import (
	"fmt"

	"github.com/c-bata/go-prompt"
	simplepromt "github.com/julienroland/copro/prompt"
)

var (
	PROMPT_PREFIX = ">>> "
)

type Cli struct {
	Commands chan string
	Suggests []prompt.Suggest

	buffer string
}

func (c *Cli) executor(in string) {
	c.Commands <- in
}

func (c *Cli) completer(in prompt.Document) []prompt.Suggest {
	c.buffer = in.Text
	return prompt.FilterHasPrefix(c.Suggests, in.GetWordBeforeCursor(), true)
}

func (c *Cli) Println(in string) {
	fmt.Printf("\033[2K\r%s\n", in)
	fmt.Print(PROMPT_PREFIX, c.buffer)
}

func GetCli() (*Cli, *prompt.Prompt) {
	c := &Cli{
		Commands: make(chan string),
		Suggests: make([]prompt.Suggest, 0),
		buffer:   "",
	}
	p := prompt.New(
		c.executor,
		c.completer,
		prompt.OptionPrefix(PROMPT_PREFIX),
	)
	return c, p
}

func Choice(prefix string, choices []string) (string, error) {
	ask := simplepromt.NewSelect()
	ask.Question = prefix
	convertedChoices := make([]*simplepromt.Choice, len(choices))
	for i := range convertedChoices {
		convertedChoices[i] = &simplepromt.Choice{
			ID:    i,
			Label: choices[i],
		}
	}
	ask.Choices = convertedChoices
	result, err := ask.Run()
	return result.Label, err
}

func Input(prefix string, dflt string, validator func(string) bool, getError func(string) string) (string, error) {
	ask := simplepromt.NewInput()
	ask.Question = prefix
	ask.Default = dflt
	ask.Validation = validator
	ask.ErrorMessage = getError
	result, err := ask.Run()
	return result, err
}
