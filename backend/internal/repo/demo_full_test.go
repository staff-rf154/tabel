package repo

import "testing"

func TestDemoLoginAddsNameLettersForUniqueness(t *testing.T) {
	used := make(map[string]bool, 4)
	if got := demoLogin("Иванов Иван Иванович", used); got != "ИвановИ" {
		t.Fatalf("first login = %q, want ИвановИ", got)
	}
	if got := demoLogin("Иванов Игнат Петрович", used); got != "ИвановИг" {
		t.Fatalf("second login = %q, want ИвановИг", got)
	}
	if got := demoLogin("Иванов Илья Сергеевич", used); got != "ИвановИл" {
		t.Fatalf("third login = %q, want ИвановИл", got)
	}
	if got := demoLogin("Иванов Иван Иванович", used); got != "ИвановИв" {
		t.Fatalf("fourth login = %q, want ИвановИв", got)
	}
	if got := demoLogin("Иванов Иван Иванович", used); got != "ИвановИва" {
		t.Fatalf("fifth login = %q, want ИвановИва", got)
	}
	if got := demoLogin("Иванов Иван Иванович", used); got != "ИвановИван" {
		t.Fatalf("exhausted login = %q, want ИвановИван", got)
	}
	if got := demoLogin("Иванов Иван Иванович", used); got != "ИвановИ2" {
		t.Fatalf("numeric fallback = %q, want ИвановИ2", got)
	}
}
