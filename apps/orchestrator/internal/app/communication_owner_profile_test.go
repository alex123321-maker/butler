package app

import "testing"

func TestCommunicationOwnerProfileDoesNotWeakenLabDatabaseGuard(t *testing.T) {
	lab := TelegramCommunicationOptions{}
	if lab.databaseAllowed("butler") || !lab.databaseAllowed("butler_test") {
		t.Fatal("lab restriction changed")
	}
	owner := TelegramCommunicationOptions{ownerDatabase: "butler"}
	if !owner.databaseAllowed("butler") || owner.databaseAllowed("other_test") || owner.databaseAllowed("other") {
		t.Fatal("operator database pin lost")
	}
}
