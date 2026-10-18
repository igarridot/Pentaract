package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// composeServices lists the top-level services of docker-compose.yml.
func composeServices(t *testing.T, compose string) map[string]bool {
	t.Helper()
	services := map[string]bool{}
	inServices := false
	for _, line := range strings.Split(compose, "\n") {
		switch {
		case line == "services:":
			inServices = true
		case inServices && line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#"):
			inServices = false
		case inServices:
			if m := regexp.MustCompile(`^  ([a-z0-9-]+):$`).FindStringSubmatch(line); m != nil {
				services[m[1]] = true
			}
		}
	}
	return services
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(mustGetwd(t), "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// A Makefile target naming a service that docker-compose.yml no longer has
// fails with "no such service" (make dev-up did, after init-perms went).
func TestMakefileUsesExistingComposeServices(t *testing.T) {
	services := composeServices(t, readRepoFile(t, "docker-compose.yml"))
	if !services["dev"] || !services["db"] {
		t.Fatalf("could not read the compose services, got %v", services)
	}

	upRe := regexp.MustCompile(`\$\(COMPOSE\)[^\n]* up -d ([a-z0-9 -]+)`)
	runRe := regexp.MustCompile(`\$\(COMPOSE\)[^\n]* run --rm (?:--[a-z-]+ )*([a-z0-9-]+)`)
	makefile := readRepoFile(t, "Makefile")

	var used []string
	for _, m := range upRe.FindAllStringSubmatch(makefile, -1) {
		for _, name := range strings.Fields(m[1]) {
			if !strings.HasPrefix(name, "-") {
				used = append(used, name)
			}
		}
	}
	for _, m := range runRe.FindAllStringSubmatch(makefile, -1) {
		used = append(used, m[1])
	}
	if len(used) == 0 {
		t.Fatalf("found no compose services in the Makefile")
	}
	for _, name := range used {
		if !services[name] {
			t.Errorf("Makefile uses compose service %q, which docker-compose.yml does not define", name)
		}
	}
}

// Every persistent_data bind mount must be created by make data-dirs, or by
// make certs for the certificate directory.
func TestMakefileCreatesBindMountedDataDirs(t *testing.T) {
	makefile := readRepoFile(t, "Makefile")
	m := regexp.MustCompile(`(?m)^DATA_DIRS := (.+)$`).FindStringSubmatch(makefile)
	if m == nil {
		t.Fatalf("Makefile defines no DATA_DIRS")
	}
	created := map[string]bool{"persistent_data/certs": true}
	for _, dir := range strings.Fields(m[1]) {
		created[dir] = true
	}

	sources := regexp.MustCompile(`source: \./(persistent_data/[a-z0-9-]+)`).FindAllStringSubmatch(readRepoFile(t, "docker-compose.yml"), -1)
	if len(sources) == 0 {
		t.Fatalf("found no persistent_data bind mounts in docker-compose.yml")
	}
	for _, src := range sources {
		if !created[src[1]] {
			t.Errorf("docker-compose.yml bind-mounts %s, which make data-dirs does not create", src[1])
		}
	}
}
