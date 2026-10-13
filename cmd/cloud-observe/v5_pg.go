package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/butler/butler/internal/acceptance"
)

func runCloudV5PGCorrelation(reportPath, witnessPath, output string, stdout io.Writer) error {
	if reportPath == "" || witnessPath == "" {
		return errors.New("separate collector report and read-only PG witness required")
	}
	report, err := acceptance.ReadPrivateFile(reportPath, acceptance.CloudV5WindowReportLimit)
	if err != nil {
		return err
	}
	witness, err := acceptance.ReadPrivateFile(witnessPath, acceptance.CloudV5WindowReportLimit)
	if err != nil {
		return err
	}
	r, err := acceptance.CorrelateCloudV5PGWitness(report, witness)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if output == "" {
		_, err = stdout.Write(b)
		return err
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
