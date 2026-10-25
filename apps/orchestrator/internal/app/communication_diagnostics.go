package app

import (
	"context"
	"net/http"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
)

func (c *telegramCommunication) diagnosticProfile() communication.DiagnosticProfile {
	p := communication.DiagnosticProfile{OwnerID: c.binding.OwnerID, DrainOnly: c.draining,
		Methods:  []string{"communication.message", "communication.photo", "communication.report"},
		Bindings: []communication.Binding{c.policy.binding}, ToolProbe: c.toolProbe}
	if c.widgets {
		p.Methods = append(p.Methods, "communication.widget")
	}
	if c.web != nil {
		p.Bindings = append(p.Bindings, c.web.binding)
	}
	return p
}

func communicationToolProbe(client *tools.BrokerClient) func(context.Context) (map[string]bool, error) {
	if client == nil {
		return nil
	}
	return func(ctx context.Context) (map[string]bool, error) {
		out := map[string]bool{}
		for _, method := range []string{"communication.message", "communication.photo", "communication.report", "communication.widget"} {
			actual, err := client.GetToolContract(ctx, method)
			if err != nil {
				return nil, communication.ErrDenied
			}
			got, e1 := actionauth.ContractHash(actual)
			want, e2 := actionauth.ContractHash(communicationcontrol.Contract(method))
			out[method] = e1 == nil && e2 == nil && got == want
		}
		return out, nil
	}
}

func (c *telegramCommunication) CommunicationReport(ctx context.Context) (communication.DiagnosticReport, error) {
	if c == nil || c.store == nil {
		return communication.DiagnosticReport{}, communication.ErrDenied
	}
	return c.store.Diagnostics(ctx, c.diagnosticProfile())
}

func (c *telegramCommunication) doctorReporter() api.CommunicationDoctorReporter {
	if c == nil {
		return nil
	}
	return c
}

func (c *telegramCommunication) diagnosticHandler(auth *api.OwnerControlAuth) http.Handler {
	if c == nil {
		return nil
	}
	return api.CommunicationDiagnostics(auth, c)
}
