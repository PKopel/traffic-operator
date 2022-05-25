package utils

import (
	"io"
	"io/ioutil"
	"strings"
)

type Metric struct {
	Name   string
	Labels map[string]string
	Value  string
}

func ParseLine(line string) Metric {
	labels := make(map[string]string)
	var name string
	var value string

	if strings.Contains(line, "{") {
		split := strings.Split(line, "{")
		name = split[0]
		rest := split[1]
		split = strings.Split(rest, "} ")
		value = split[1]

		for _, label := range strings.Split(split[0], ",") {
			pair := strings.Split(label, "=")
			labels[pair[0]] = pair[1]
		}
	} else {
		split := strings.Split(line, " ")
		name = split[0]
		value = split[1]
	}

	return Metric{
		Name:   name,
		Labels: labels,
		Value:  value,
	}
}

func ParseAll(metrics io.ReadCloser) ([]Metric, error) {
	defer metrics.Close()

	body, err := ioutil.ReadAll(metrics)
	if err != nil {
		return nil, err
	}

	text := string(body)

	lines := strings.Split(text, "\n")

	lines = Filter(lines, func(line string) bool {
		return !strings.Contains(line, "#") && strings.ContainsAny(line, "_={}\"")
	})

	result := make([]Metric, len(lines))

	for i, line := range lines {
		result[i] = ParseLine(line)
	}

	return result, nil
}
