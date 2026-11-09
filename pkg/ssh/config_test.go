package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skevetter/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SSHConfigTestSuite struct {
	suite.Suite
}

func TestSSHConfigSuite(t *testing.T) {
	suite.Run(t, new(SSHConfigTestSuite))
}

func (s *SSHConfigTestSuite) TestAddHostSection() {
	tests := []struct {
		name       string
		config     string
		execPath   string
		host       string
		user       string
		context    string
		workspace  string
		workdir    string
		command    string
		gpgagent   bool
		devPodHome string
		provider   string
		expected   string
	}{
		{
			name:       "Basic host addition",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "AWS provider with ConnectTimeout",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "aws",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ConnectTimeout 60
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Basic host addition with DEVPOD_HOME",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "C:\\\\White Space\\devpod\\test",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace --devpod-home "C:\\White Space\devpod\test"
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Host addition with workdir",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "/path/to/workdir",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace --workdir "/path/to/workdir"
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Host addition with gpg agent",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   true,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace --gpg-agent-forwarding
  User testuser
# DevPod End testhost`,
		},
		{
			name:       "Host addition with custom command",
			config:     "",
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "ssh -W %h:%p bastion",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "ssh -W %h:%p bastion"
  User testuser
# DevPod End testhost`,
		},
		{
			name: "Host addition to existing config",
			config: `Host existinghost
  User existinguser`,
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost
Host existinghost
  User existinguser`,
		},
		{
			name: "Host addition to existing config with DevPod host",
			config: "# DevPod Start existingtesthost\n" +
				"Host existingtesthost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End existingtesthost\n\n" +
				"Host existinghost\n" +
				"  User existinguser",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"# DevPod Start existingtesthost\n" +
				"Host existingtesthost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End existingtesthost\n\n" +
				"Host existinghost\n" +
				"  User existinguser",
		},
		{
			name:      "Host addition with lowercase host entries",
			config:    "host 192.168.1.1\n  User alice\n  Port 22\n\nhost myserver\n  User bob",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"host 192.168.1.1\n  User alice\n  Port 22\n\nhost myserver\n  User bob",
		},
		{
			name:      "Host addition with uppercase HOST entries",
			config:    "HOST 192.168.1.1\n  User alice\n  Port 22\n\nHOST myserver\n  User bob",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"HOST 192.168.1.1\n  User alice\n  Port 22\n\nHOST myserver\n  User bob",
		},
		{
			name:      "Host addition with mixed case HoSt entries",
			config:    "HoSt 192.168.1.1\n  User alice\n  Port 22\n\nHoSt myserver\n  User bob",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"HoSt 192.168.1.1\n  User alice\n  Port 22\n\nHoSt myserver\n  User bob",
		},
		{
			name:      "Host addition with tab separated Host keyword",
			config:    "Host\t192.168.1.1\n  User alice\n  Port 22\n\nHost\tmyserver\n  User bob",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"Host\t192.168.1.1\n  User alice\n  Port 22\n\nHost\tmyserver\n  User bob",
		},
		{
			name:      "Host addition with equals separated Host keyword",
			config:    "Host=192.168.1.1\n  User alice\n  Port 22\n\nHost=myserver\n  User bob",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"Host=192.168.1.1\n  User alice\n  Port 22\n\nHost=myserver\n  User bob",
		},
		{
			name:      "Host addition before Match section",
			config:    "Match exec \"true\"\n  User conditional\n  Port 22",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"Match exec \"true\"\n  User conditional\n  Port 22",
		},
		{
			name: "Host addition does not corrupt existing DevPod block followed by plain host",
			config: "host plain-host\n  User alice\n\n" +
				"# DevPod Start existing.devpod\n" +
				"Host existing.devpod\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context ctx --user user existing\n" +
				"  User user\n" +
				"# DevPod End existing.devpod",
			execPath:  "/path/to/exec",
			host:      "testhost",
			user:      "testuser",
			context:   "testcontext",
			workspace: "testworkspace",
			expected: "# DevPod Start testhost\n" +
				"Host testhost\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context testcontext --user testuser testworkspace\n" +
				"  User testuser\n" +
				"# DevPod End testhost\n" +
				"host plain-host\n  User alice\n\n" +
				"# DevPod Start existing.devpod\n" +
				"Host existing.devpod\n" +
				"  ForwardAgent yes\n" +
				"  LogLevel error\n" +
				"  StrictHostKeyChecking no\n" +
				"  UserKnownHostsFile /dev/null\n" +
				"  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa\n" +
				"  ProxyCommand \"/path/to/exec\" ssh --stdio" +
				" --context ctx --user user existing\n" +
				"  User user\n" +
				"# DevPod End existing.devpod",
		},
		{
			name: "Host addition after top level includes",
			config: `Include ~/config1

Include ~/config2



Include ~/config3`,
			execPath:   "/path/to/exec",
			host:       "testhost",
			user:       "testuser",
			context:    "testcontext",
			workspace:  "testworkspace",
			workdir:    "",
			command:    "",
			gpgagent:   false,
			devPodHome: "",
			provider:   "",
			expected: `Include ~/config1

Include ~/config2



Include ~/config3
# DevPod Start testhost
Host testhost
  ForwardAgent yes
  LogLevel error
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  HostKeyAlgorithms rsa-sha2-256,rsa-sha2-512,ssh-rsa
  ProxyCommand "/path/to/exec" ssh --stdio --context testcontext --user testuser testworkspace
  User testuser
# DevPod End testhost`,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			result, err := addHostSection(tt.config, tt.execPath, addHostParams{
				path:       "",
				host:       tt.host,
				user:       tt.user,
				context:    tt.context,
				workspace:  tt.workspace,
				workdir:    tt.workdir,
				command:    tt.command,
				gpgagent:   tt.gpgagent,
				devPodHome: tt.devPodHome,
				provider:   tt.provider,
			})

			assert.NoError(s.T(), err)
			assert.Equal(s.T(), tt.expected, result)
			assert.Contains(s.T(), result, MarkerEndPrefix+tt.host)
			assert.Contains(s.T(), result, "Host "+tt.host)
			assert.Contains(s.T(), result, "User "+tt.user)

			if tt.command != "" {
				assert.Contains(s.T(), result, "ProxyCommand \""+tt.command+"\"")
			}

			if tt.workdir != "" {
				assert.Contains(s.T(), result, "--workdir \""+tt.workdir+"\"")
			}

			if tt.gpgagent {
				assert.Contains(s.T(), result, "--gpg-agent-forwarding")
			}

			if tt.config != "" {
				assert.Contains(s.T(), result, tt.config)
			}

			if strings.Contains(tt.config, MarkerStartPrefix) {
				idxStart := strings.Index(tt.config, MarkerStartPrefix)
				lineEnd := strings.Index(tt.config[idxStart:], "\n")
				var existingStartMarker string
				if lineEnd != -1 {
					existingStartMarker = tt.config[idxStart : idxStart+lineEnd]
				} else {
					existingStartMarker = tt.config[idxStart:]
				}
				assert.Contains(s.T(), result, existingStartMarker)
				assert.Less(
					s.T(),
					strings.Index(result, MarkerStartPrefix+tt.host),
					strings.Index(result, existingStartMarker),
				)
			}
		})
	}
}

func (s *SSHConfigTestSuite) TestSSHConfigKeyword() {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "", expected: ""},
		{input: "# comment", expected: ""},
		{input: "  # indented comment", expected: ""},
		{input: "Host myserver", expected: "Host"},
		{input: "host myserver", expected: "host"},
		{input: "HOST myserver", expected: "HOST"},
		{input: "HoSt\tmyserver", expected: "HoSt"},
		{input: "Host=myserver", expected: "Host"},
		{input: "HostName example.com", expected: "HostName"},
		{input: "Match exec \"true\"", expected: "Match"},
		{input: "match host foo", expected: "match"},
		{input: "Port 22", expected: "Port"},
	}

	for _, tt := range tests {
		assert.Equal(s.T(), tt.expected, sshConfigKeyword(tt.input))
	}
}

func (s *SSHConfigTestSuite) TestIsSSHSectionStart() {
	tests := []struct {
		input    string
		expected bool
	}{
		{input: "Host myserver", expected: true},
		{input: "host myserver", expected: true},
		{input: "HOST myserver", expected: true},
		{input: "HoSt\tmyserver", expected: true},
		{input: "Host=myserver", expected: true},
		{input: "Match exec \"true\"", expected: true},
		{input: "match host foo", expected: true},
		{input: "MATCH all", expected: true},
		{input: "HostName example.com", expected: false},
		{input: "Hostname example.com", expected: false},
		{input: "hostname example.com", expected: false},
		{input: "Port 22", expected: false},
		{input: "# Host commented", expected: false},
		{input: "", expected: false},
	}

	for _, tt := range tests {
		assert.Equal(s.T(), tt.expected, isSSHSectionStart(tt.input))
	}
}

func (s *SSHConfigTestSuite) TestFindInsertPosition() {
	s.Run("does not treat HostName as section start", func() {
		config := "HostName global.example.com\nHost actual-host\n  User example"
		pos, lines, err := findInsertPosition(config)
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 1, pos)
		assert.Equal(s.T(), 3, len(lines))
	})

	s.Run("inserts before Match section with preceding options", func() {
		config := "IdentityFile ~/.ssh/id_ed25519\n\nMatch exec \"true\"\n  User conditional"
		pos, _, err := findInsertPosition(config)
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 2, pos)
	})

	s.Run("inserts before existing DevPod block including start marker", func() {
		config := "# DevPod Start existing\nHost existing\n  User user\n# DevPod End existing"
		pos, _, err := findInsertPosition(config)
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 0, pos)
	})

	s.Run("inserts before comments attached to host", func() {
		config := "# Server comment\nHost actual-host\n  User example"
		pos, _, err := findInsertPosition(config)
		assert.NoError(s.T(), err)
		assert.Equal(s.T(), 0, pos)
	})
}

func (s *SSHConfigTestSuite) TestConfigureSSHConfig() {
	tmpDir := s.T().TempDir()
	sshConfigFile := filepath.Join(tmpDir, "config")

	existingConfig := "host lowercase-host\n" +
		"  User alice\n\n" +
		"# DevPod Start existing\n" +
		"Host existing\n" +
		"  User existinguser\n" +
		"# DevPod End existing\n\n" +
		"Match exec \"true\"\n" +
		"  User conditional\n"

	err := os.WriteFile(sshConfigFile, []byte(existingConfig), 0o600)
	assert.NoError(s.T(), err)

	err = ConfigureSSHConfig(SSHConfigParams{
		SSHConfigPath: sshConfigFile,
		Workspace:     "myworkspace",
		User:          "newuser",
		Context:       "testcontext",
		Log:           log.Discard,
	})
	assert.NoError(s.T(), err)

	content, err := os.ReadFile(sshConfigFile) // #nosec G304 -- test path from t.TempDir
	assert.NoError(s.T(), err)
	result := string(content)

	newHost := "myworkspace.devpod"
	assert.Contains(s.T(), result, MarkerStartPrefix+newHost)
	assert.Contains(s.T(), result, MarkerEndPrefix+newHost)
	assert.Contains(s.T(), result, "Host "+newHost)
	assert.Contains(s.T(), result, "User newuser")

	existingBlock := "# DevPod Start existing\n" +
		"Host existing\n" +
		"  User existinguser\n" +
		"# DevPod End existing"
	assert.Contains(s.T(), result, existingBlock)

	newStartIdx := strings.Index(result, MarkerStartPrefix+newHost)
	firstSectionIdx := strings.Index(result, "host lowercase-host")
	existingStartIdx := strings.Index(result, MarkerStartPrefix+"existing")

	assert.Less(s.T(), newStartIdx, firstSectionIdx)
	assert.Less(s.T(), newStartIdx, existingStartIdx)
}
