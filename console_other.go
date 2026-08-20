//go:build !windows

package main

import (
	"bufio"
	"os"
	"time"
)

type consoleInput struct{ reader *bufio.Reader }

func openConsoleInput() (*consoleInput, error) {
	return &consoleInput{reader: bufio.NewReader(os.Stdin)}, nil
}
func (c *consoleInput) close() {}
func (c *consoleInput) readKey(_ time.Duration) (keyPress, bool) {
	r, _, err := c.reader.ReadRune()
	return keyPress{char: r}, err == nil
}
func terminalWidth() int    { return 120 }
func setConsoleTitle(_ string) {}
