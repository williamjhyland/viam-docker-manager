package docker

import (
	"context"
	"testing"

	"github.com/edaniels/golog"
	"github.com/stretchr/testify/assert"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/resource"
)

func setupDependencies() (resource.Config, resource.Dependencies) {
	cfg := resource.Config{
		Name:  "movementsensor",
		Model: Model,
		API:   sensor.API,
		ConvertedAttributes: &Config{
			ImageName:  "ubuntu",
			RepoDigest: "sha256:218bb51abbd1864df8be26166f847547b3851a89999ca7bfceb85ca9b5d2e95d",
			ComposeFile: []string{
				"services:",
				"  app:",
				"    image: ubuntu@sha256:218bb51abbd1864df8be26166f847547b3851a89999ca7bfceb85ca9b5d2e95d",
				"    command: sleep 10",
				"    working_dir: /root",
			},
		},
	}

	return cfg, resource.Dependencies{}
}

func TestReconfigureWritesDockerComposeFile(t *testing.T) {
	cfg, deps := setupDependencies()
	sensor, err := NewDockerSensor(context.Background(), deps, cfg, golog.NewTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, sensor)
}

func TestImageStarts(t *testing.T) {
	cfg, deps := setupDependencies()
	sensor, err := NewDockerSensor(context.Background(), deps, cfg, golog.NewTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}

	// Make sure we created the sensor
	assert.NotNil(t, sensor)

	// Now make sure it is actually running
	dm := LocalDockerManager{}
	containers, err := dm.ListContainers()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(containers))

	sensor.Close(context.Background())
}

func TestCleanupOldImage(t *testing.T) {
	cfg, deps := setupDependencies()
	sensor, err := NewDockerSensor(context.Background(), deps, cfg, golog.NewTestLogger(t))
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, sensor)

	dm := LocalDockerManager{}
	images, err := dm.ListImages()
	assert.NoError(t, err)
	if len(images) != 1 {
		t.Logf("Found %d images, expected 1", len(images))
		for _, image := range images {
			t.Logf("Image: %#v", image)
		}
		t.FailNow()
	}
	assert.Equal(t, 1, len(images))

	newConfig, _ := setupDependencies()
	newConfig.ConvertedAttributes.(*Config).RepoDigest = "sha256:c9cf959fd83770dfdefd8fb42cfef0761432af36a764c077aed54bbc5bb25368"
	newConfig.ConvertedAttributes.(*Config).ComposeFile = []string{
		"services:",
		"  app:",
		"    image: ubuntu@sha256:c9cf959fd83770dfdefd8fb42cfef0761432af36a764c077aed54bbc5bb25368",
		"    command: sleep 10",
		"    working_dir: /root",
	}
	sensor.Reconfigure(context.Background(), deps, newConfig)

	images, err = dm.ListImages()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(images))
	assert.Equal(t, "sha256:c9cf959fd83770dfdefd8fb42cfef0761432af36a764c077aed54bbc5bb25368", images[0].RepoDigest)

	err = dm.RemoveImageByRepoDigest("sha256:c9cf959fd83770dfdefd8fb42cfef0761432af36a764c077aed54bbc5bb25368")
	assert.NoError(t, err)
}
