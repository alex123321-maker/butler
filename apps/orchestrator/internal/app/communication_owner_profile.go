package app

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/butler/butler/internal/communicationcontrol"
)

// NewForOwnerCommunication enables qualified message/photo/report and optional widget
// profile for the explicitly pinned owner. Existing bindings and historical
// admissions are never upgraded. The lab constructor retains its separate *_test guard.
func NewForOwnerCommunication(ctx context.Context, p communicationcontrol.OwnerProfile) (*App, error) {
	if p.Validate() != nil {
		return nil, communicationcontrol.ErrDenied
	}
	if _, err := communicationModelProxy(os.Getenv("HTTPS_PROXY"), http.ProxyFromEnvironment); err != nil {
		return nil, communicationcontrol.ErrDenied
	}
	return newApp(ctx, &TelegramCommunicationOptions{
		OwnerID: p.OwnerID, BindingID: p.BindingID, ExpectedBot: p.ExpectedBot,
		TokenFile: p.TokenFile, BotAPIProxyFile: p.BotAPIProxyFile,
		ChatID: p.ChatID, BindingVersion: p.BindingVersion,
		WebBindingID: p.WebBindingID, WebBindingVersion: p.WebBindingVersion,
		DrainOnly: p.Mode == "drain", AllowPrivate: p.AllowPrivate,
		Widgets: p.Widgets, EmojiDecoration: p.EmojiDecoration,
		MaxOpenInteractions: p.MaxOpenInteractions,
		ownerDatabase:       p.Database, expectedBotID: p.BotID, initialUpdateID: p.InitialUpdateID,
	})
}

func (o TelegramCommunicationOptions) databaseAllowed(database string) bool {
	if o.ownerDatabase != "" {
		return database == o.ownerDatabase
	}
	return strings.HasSuffix(database, "_test")
}
