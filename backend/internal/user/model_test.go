package user

import "testing"

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"active is valid", StatusActive, true},
		{"deactivated is valid", StatusDeactivated, true},
		{"suspended is valid", StatusSuspended, true},
		{"empty is invalid", Status(""), false},
		{"unknown is invalid", Status("deleted"), false},
		{"uppercase is invalid", Status("ACTIVE"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsValid()
			if got != tt.want {
				t.Errorf("Status(%q).IsValid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestUser_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"active user", StatusActive, true},
		{"deactivated user", StatusDeactivated, false},
		{"suspended user", StatusSuspended, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{Status: tt.status}
			got := u.IsActive()
			if got != tt.want {
				t.Errorf("User{Status: %q}.IsActive() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestListFilter_Offset(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		want     int
	}{
		{"page 1", 1, 20, 0},
		{"page 2", 2, 20, 20},
		{"page 3 with size 10", 3, 10, 20},
		{"page 0 returns 0", 0, 20, 0},
		{"negative page returns 0", -1, 20, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ListFilter{Page: tt.page, PageSize: tt.pageSize}
			got := f.Offset()
			if got != tt.want {
				t.Errorf("ListFilter{Page: %d, PageSize: %d}.Offset() = %d, want %d",
					tt.page, tt.pageSize, got, tt.want)
			}
		})
	}
}
