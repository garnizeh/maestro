package beam

import (
	"testing"
)

type portMappingTestCase struct {
	name      string
	spec      string
	wantErr   bool
	wantCount int
	wantFirst PortMapping
}

func runPortMappingTest(t *testing.T, tc portMappingTestCase) {
	t.Helper()
	got, err := ParsePortMapping(tc.spec)
	if tc.wantErr {
		if err == nil {
			t.Errorf("ParsePortMapping(%q) expected error but got nil (result: %v)", tc.spec, got)
		}
		return
	}
	if err != nil {
		t.Fatalf("ParsePortMapping(%q) unexpected error: %v", tc.spec, err)
	}
	if len(got) != tc.wantCount {
		t.Errorf("ParsePortMapping(%q) got %d mappings, want %d", tc.spec, len(got), tc.wantCount)
	}
	if len(got) > 0 {
		first := got[0]
		if first.HostPort != tc.wantFirst.HostPort {
			t.Errorf("HostPort: got %d, want %d", first.HostPort, tc.wantFirst.HostPort)
		}
		if first.ContainerPort != tc.wantFirst.ContainerPort {
			t.Errorf(
				"ContainerPort: got %d, want %d",
				first.ContainerPort,
				tc.wantFirst.ContainerPort,
			)
		}
		if first.Protocol != tc.wantFirst.Protocol {
			t.Errorf("Protocol: got %q, want %q", first.Protocol, tc.wantFirst.Protocol)
		}
		if first.HostIP != tc.wantFirst.HostIP {
			t.Errorf("HostIP: got %q, want %q", first.HostIP, tc.wantFirst.HostIP)
		}
	}
}

func TestParsePortMapping_Basic(t *testing.T) {
	t.Parallel()
	tests := []portMappingTestCase{
		{
			name:      "simple host:container",
			spec:      "8080:80",
			wantCount: 1,
			wantFirst: PortMapping{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
		{
			name:      "port 65535 valid",
			spec:      "65535:65535",
			wantCount: 1,
			wantFirst: PortMapping{HostPort: 65535, ContainerPort: 65535, Protocol: "tcp"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runPortMappingTest(t, tc)
		})
	}
}

func TestParsePortMapping_Protocols(t *testing.T) {
	t.Parallel()
	tests := []portMappingTestCase{
		{
			name:      "with tcp suffix",
			spec:      "8080:80/tcp",
			wantCount: 1,
			wantFirst: PortMapping{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
		{
			name:      "udp protocol",
			spec:      "5353:53/udp",
			wantCount: 1,
			wantFirst: PortMapping{HostPort: 5353, ContainerPort: 53, Protocol: "udp"},
		},
		{
			name:      "sctp protocol",
			spec:      "9999:9999/sctp",
			wantCount: 1,
			wantFirst: PortMapping{HostPort: 9999, ContainerPort: 9999, Protocol: "sctp"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runPortMappingTest(t, tc)
		})
	}
}

func TestParsePortMapping_IPs(t *testing.T) {
	t.Parallel()
	tests := []portMappingTestCase{
		{
			name:      "with host IP",
			spec:      "127.0.0.1:8080:80",
			wantCount: 1,
			wantFirst: PortMapping{
				HostPort:      8080,
				ContainerPort: 80,
				Protocol:      "tcp",
				HostIP:        "127.0.0.1",
			},
		},
		{
			name:      "with host IP and udp",
			spec:      "127.0.0.1:5353:53/udp",
			wantCount: 1,
			wantFirst: PortMapping{
				HostPort:      5353,
				ContainerPort: 53,
				Protocol:      "udp",
				HostIP:        "127.0.0.1",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runPortMappingTest(t, tc)
		})
	}
}

func TestParsePortMapping_Ranges(t *testing.T) {
	t.Parallel()
	tests := []portMappingTestCase{
		{
			name:      "port range equal length",
			spec:      "8080-8082:80-82",
			wantCount: 3,
			wantFirst: PortMapping{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
		{
			name:      "port range single container port (fan-in)",
			spec:      "8080-8082:80",
			wantCount: 3,
			wantFirst: PortMapping{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runPortMappingTest(t, tc)
		})
	}
}

func TestParsePortMapping_Errors(t *testing.T) {
	t.Parallel()
	tests := []portMappingTestCase{
		{
			name:    "invalid protocol",
			spec:    "8080:80/xyz",
			wantErr: true,
		},
		{
			name:    "invalid one-part format",
			spec:    "invalid",
			wantErr: true,
		},
		{
			name:    "invalid host IP",
			spec:    "notanip:8080:80",
			wantErr: true,
		},
		{
			name:    "port out of range (high)",
			spec:    "99999:80",
			wantErr: true,
		},
		{
			name:    "port zero",
			spec:    "0:80",
			wantErr: true,
		},
		{
			name:    "container port out of range",
			spec:    "8080:99999",
			wantErr: true,
		},
		{
			name:    "unequal ranges",
			spec:    "8080-8082:80-81",
			wantErr: true,
		},
		{
			name:    "inverted range",
			spec:    "8082-8080:80",
			wantErr: true,
		},
		{
			name:    "port 65536 invalid",
			spec:    "65536:80",
			wantErr: true,
		},
		{
			name:    "empty host port",
			spec:    ":80",
			wantErr: true,
		},
		{
			name:    "empty container port",
			spec:    "8080:",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runPortMappingTest(t, tc)
		})
	}
}

// TestParsePortRangeEdgeCases tests internal parsePortRange directly through ParsePortMapping.
func TestParsePortRangeEdgeCases(t *testing.T) {
	t.Parallel()

	// Malformed range string with more than 2 parts (triggers len != 2 branch)
	_, err := ParsePortMapping("8080-8081-8082:80")
	if err == nil {
		t.Error("expected error for multi-dash port range, got nil")
	}

	// Non-numeric port in range triggers atoi failure
	_, err = ParsePortMapping("8080-808a:80")
	if err == nil {
		t.Error("expected error for non-numeric port in range, got nil")
	}
}
