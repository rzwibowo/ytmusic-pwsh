//go:build !windows

package main

func listenMediaKeys(commands chan<- string) {}