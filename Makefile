# Makefile for nmcleaner

.PHONY: all build run clean

all: build

build:
	go build -o nmcleaner .

run: build
	./nmcleaner

clean:
	go clean
	rm -f nmcleaner
