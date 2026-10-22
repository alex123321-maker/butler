// Package telegramrich implements the opt-in Telegram communication adapter.
// It is separate from the legacy adapter and is not installed by Bootstrap.
package telegramrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

var ErrUnavailable = errors.New("Telegram rich delivery unavailable or outside policy")

const maxPartBytes = 32 * 1024 // Local bound; not a claimed Telegram API limit.
const maxPartMediaBytes = 20 * 1024 * 1024

type File struct {
	Ref    string `json:"artifact_ref"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
	Field  string `json:"field"`
}
type Caption struct {
	Text string `json:"text"`
}
type InputMedia struct {
	Type  string `json:"type"`
	Media string `json:"media"`
}
type Cell struct {
	Text   string `json:"text"`
	Header bool   `json:"is_header,omitempty"`
	Align  string `json:"align"`
	VAlign string `json:"valign"`
}
type ListItem struct {
	Blocks []Block `json:"blocks"`
}
type Block struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	Size     int         `json:"size,omitempty"`
	Blocks   []Block     `json:"blocks,omitempty"`
	Items    []ListItem  `json:"items,omitempty"`
	Cells    [][]Cell    `json:"cells,omitempty"`
	Photo    *InputMedia `json:"photo,omitempty"`
	Document *InputMedia `json:"document,omitempty"`
	Caption  *Caption    `json:"caption,omitempty"`
}
type RichMessage struct {
	Blocks []Block `json:"blocks"`
}

// Part is a private immutable outbox manifest. No file bytes, URLs or secrets.
type Part struct {
	Version int          `json:"version"`
	Method  string       `json:"method"`
	Text    string       `json:"text,omitempty"`
	Photo   string       `json:"photo,omitempty"`
	Caption string       `json:"caption,omitempty"`
	Rich    *RichMessage `json:"rich_message,omitempty"`
	Files   []File       `json:"files,omitempty"`
}

type MediaResolver func(context.Context, pgx.Tx, string, communication.Binding, string) (artifacts.Media, error)
type Planner struct {
	Resolve MediaResolver
	Profile Capabilities
}

// Capabilities are trusted adapter readiness, never a model enable flag.
// A zero profile exposes nothing. Reports require the complete native mapping.
type Capabilities struct{ Message, Photo, Rich, Document bool }

func (c Capabilities) Methods() []string {
	methods := []string{}
	if c.Message {
		methods = append(methods, "communication.message")
	}
	if c.Message && c.Photo {
		methods = append(methods, "communication.photo")
	}
	if c.Rich && c.Photo && c.Document {
		methods = append(methods, "communication.report")
	}
	return methods
}

// Plan is called inside the owning publication transaction, after ChannelGuard.
// Resolve must enforce current retained-artifact and per-channel export policy.
func (p Planner) Plan(ctx context.Context, tx pgx.Tx, run string, binding communication.Binding, args communicationcontrol.Args) ([]string, error) {
	if binding.Channel != "telegram" {
		return nil, ErrUnavailable
	}
	if args.Text != "" && !p.Profile.Message || len(args.Images) > 0 && !p.Profile.Photo || args.Title != "" && (!p.Profile.Rich || !p.Profile.Photo || !p.Profile.Document) {
		return nil, ErrUnavailable
	}
	var parts []Part
	mediaIndex := 0
	file := func(ref string, photo bool) (File, error) {
		if p.Resolve == nil {
			return File{}, ErrUnavailable
		}
		m, err := p.Resolve(ctx, tx, run, binding, ref)
		if err != nil || m.Ref != ref || m.Validate() != nil || photo && !strings.HasPrefix(m.MIME, "image/") {
			return File{}, ErrUnavailable
		}
		mediaIndex++
		return File{Ref: ref, Name: m.Name, MIME: m.MIME, SHA256: m.SHA256, Size: len(m.Data), Field: fmt.Sprintf("file%d", mediaIndex)}, nil
	}
	if args.Text != "" {
		for _, text := range deliveryoutbox.Split(args.Text, 4096) {
			parts = append(parts, Part{Version: 1, Method: "sendMessage", Text: text})
		}
	}
	if len(args.Images) > 0 {
		for _, im := range args.Images {
			f, err := file(im.ArtifactRef, true)
			if err != nil {
				return nil, err
			}
			caption := im.Caption
			if caption == "" {
				caption = im.Alt
			}
			if len(deliveryoutbox.Split(caption, 1024)) > 1 {
				return nil, ErrUnavailable
			}
			parts = append(parts, Part{Version: 1, Method: "sendPhoto", Photo: "attach://" + f.Field, Caption: caption, Files: []File{f}})
		}
	}
	if args.Title != "" {
		current := Part{Version: 1, Method: "sendRichMessage", Rich: &RichMessage{}}
		flush := func() {
			if len(current.Rich.Blocks) > 0 {
				parts = append(parts, current)
			}
			current = Part{Version: 1, Method: "sendRichMessage", Rich: &RichMessage{}}
		}
		add := func(block Block, files []File) error {
			candidate := Part{Version: 1, Method: "sendRichMessage", Rich: &RichMessage{Blocks: append(append([]Block{}, current.Rich.Blocks...), block)}, Files: append(append([]File{}, current.Files...), files...)}
			raw, _ := json.Marshal(candidate)
			size := 0
			for _, f := range candidate.Files {
				size += f.Size
			}
			if len(raw) > maxPartBytes || len(candidate.Rich.Blocks) > 50 || size > maxPartMediaBytes {
				flush()
				candidate = Part{Version: 1, Method: "sendRichMessage", Rich: &RichMessage{Blocks: []Block{block}}, Files: files}
			}
			raw, _ = json.Marshal(candidate)
			size = 0
			for _, f := range candidate.Files {
				size += f.Size
			}
			if len(raw) > maxPartBytes || size > maxPartMediaBytes {
				return ErrUnavailable
			}
			current = candidate
			return nil
		}
		if add(Block{Type: "heading", Text: args.Title, Size: 1}, nil) != nil || add(Block{Type: "paragraph", Text: args.Summary}, nil) != nil {
			return nil, ErrUnavailable
		}
		for _, b := range args.Blocks {
			if !validBlockShape(b) {
				return nil, ErrUnavailable
			}
			block := Block{Type: b.Type, Text: b.Text}
			var files []File
			switch b.Type {
			case "paragraph", "divider":
			case "heading":
				block.Size = 2
			case "code":
				block.Type = "pre"
			case "quote":
				block = Block{Type: "blockquote", Blocks: []Block{{Type: "paragraph", Text: b.Text}}}
			case "list":
				block.Text = ""
				for _, item := range b.Items {
					block.Items = append(block.Items, ListItem{Blocks: []Block{{Type: "paragraph", Text: item}}})
				}
				if len(block.Items) == 0 {
					return nil, ErrUnavailable
				}
			case "table":
				block.Text = ""
				width := 0
				for r, row := range b.Rows {
					if len(row) == 0 || width != 0 && width != len(row) {
						return nil, ErrUnavailable
					}
					width = len(row)
					cells := []Cell{}
					for _, text := range row {
						cells = append(cells, Cell{Text: text, Header: r == 0, Align: "left", VAlign: "top"})
					}
					block.Cells = append(block.Cells, cells)
				}
				if width == 0 {
					return nil, ErrUnavailable
				}
			case "image", "document":
				f, err := file(b.ArtifactRef, b.Type == "image")
				if err != nil {
					return nil, err
				}
				files = []File{f}
				block.Text = ""
				block.Caption = &Caption{Text: b.Alt}
				media := &InputMedia{Type: "document", Media: "attach://" + f.Field}
				if b.Type == "image" {
					block.Type = "photo"
					media.Type = "photo"
					block.Photo = media
				} else {
					block.Document = media
				}
			default:
				return nil, ErrUnavailable
			}
			if err := add(block, files); err != nil {
				return nil, err
			}
		}
		for _, ref := range args.Attachments {
			f, err := file(ref, false)
			if err != nil {
				return nil, err
			}
			if err = add(Block{Type: "document", Document: &InputMedia{Type: "document", Media: "attach://" + f.Field}, Caption: &Caption{Text: f.Name}}, []File{f}); err != nil {
				return nil, err
			}
		}
		flush()
	}
	if len(parts) == 0 || len(parts) > 100 {
		return nil, ErrUnavailable
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		raw, err := json.Marshal(part)
		if err != nil || len(raw) > maxPartBytes {
			return nil, ErrUnavailable
		}
		out = append(out, string(raw))
	}
	return out, nil
}

// Never silently discard fields which do not belong to a report block kind.
func validBlockShape(b communicationcontrol.Block) bool {
	return communicationcontrol.ValidReportBlock(b)
}
