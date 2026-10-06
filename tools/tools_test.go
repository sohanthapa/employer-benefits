package tools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const mockData = "../docs/mock_data.json"

func loadStore(t *testing.T) *Store {
	t.Helper()
	s, err := Load(mockData)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load("does_not_exist.json"); err == nil {
		t.Fatal("want error for missing file, got nil")
	}
}

func TestLoad_InvalidData(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"malformed json", `{"plans": `},
		{"no plans", `{}`},
		{"plan missing a specialty", `{"plans": {"E1": {"coverage": {"anxiety": true}}}}`},
		{"provider with unknown specialty", `{
			"plans": {"E1": {"coverage": {"family_counseling": true, "marriage_counseling": true,
				"anxiety": true, "depression": true, "substance_use": true, "child_therapy": true}}},
			"providers": [{"name": "X", "zip_code": "94107", "specialty": "dentistry"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data.json")
			if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}

func TestGetPlanDetails(t *testing.T) {
	s := loadStore(t)

	plan, err := s.GetPlanDetails("E123")
	if err != nil {
		t.Fatalf("GetPlanDetails: %v", err)
	}
	t.Logf("E123 plan: %+v", plan)

	if !plan.Coverage["family_counseling"] {
		t.Error("E123: want family_counseling covered")
	}
	if plan.Coverage["substance_use"] {
		t.Error("E123: want substance_use not covered")
	}

	// Spaces and lower case from the LLM should still match.
	if _, err := s.GetPlanDetails(" e123 "); err != nil {
		t.Errorf(`GetPlanDetails(" e123 "): %v`, err)
	}
}

func TestGetPlanDetails_EmployeeNotFound(t *testing.T) {
	s := loadStore(t)

	_, err := s.GetPlanDetails("E999")
	if !errors.Is(err, ErrEmployeeNotFound) {
		t.Fatalf("want ErrEmployeeNotFound, got %v", err)
	}
}

func TestSearchProviders(t *testing.T) {
	s := loadStore(t)

	tests := []struct {
		name      string
		zip       string
		specialty string
		want      int
	}{
		{"providers found", "94107", "family_counseling", 2},
		{"no provider in zip", "94107", "anxiety", 0},
		{"unknown zip", "00000", "family_counseling", 0},
		{"spaces and upper case", " 94107 ", " Family_Counseling ", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.SearchProviders(tc.zip, tc.specialty)
			if err != nil {
				t.Fatalf("SearchProviders: %v", err)
			}
			t.Logf("%s / %s -> %+v", tc.zip, tc.specialty, got)
			if len(got) != tc.want {
				t.Errorf("want %d providers, got %d", tc.want, len(got))
			}
		})
	}
}

func TestSearchProviders_UnsupportedSpecialty(t *testing.T) {
	s := loadStore(t)

	_, err := s.SearchProviders("94107", "dentistry")
	if !errors.Is(err, ErrUnsupportedSpecialty) {
		t.Fatalf("want ErrUnsupportedSpecialty, got %v", err)
	}
}

func TestCreateSupportTicket(t *testing.T) {
	s := loadStore(t)

	first, err := s.CreateSupportTicket("no family counseling provider near 94107")
	if err != nil {
		t.Fatalf("CreateSupportTicket: %v", err)
	}
	second, err := s.CreateSupportTicket("employee E999 not found")
	if err != nil {
		t.Fatalf("CreateSupportTicket: %v", err)
	}
	t.Logf("tickets: %+v, %+v", first, second)

	if second.Reason != "employee E999 not found" {
		t.Errorf("unexpected reason %q", second.Reason)
	}
	if first.ID != "TICKET-001" || second.ID != "TICKET-002" {
		t.Errorf("want TICKET-001 and TICKET-002, got %s and %s", first.ID, second.ID)
	}
}

func TestCreateSupportTicket_EmptyReason(t *testing.T) {
	s := loadStore(t)

	_, err := s.CreateSupportTicket("   ")
	if !errors.Is(err, ErrEmptyReason) {
		t.Fatalf("want ErrEmptyReason, got %v", err)
	}
}
