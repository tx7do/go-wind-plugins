package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateTopic(t *testing.T) {
	skipWithoutIntegration(t)

	err := CreateTopic(defaultAddr, testTopic, 100, 1)
	assert.Nil(t, err)
}

func TestDeleteTopic(t *testing.T) {
	skipWithoutIntegration(t)

	err := DeleteTopic(defaultAddr, testTopic)
	assert.Nil(t, err)
}
