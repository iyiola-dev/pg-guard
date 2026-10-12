package analyzer

import (
	"github.com/iyiola-dev/pg-guard/internal/pgtest"
	"os"
	"testing"
)

func TestMain(m *testing.M) { code := m.Run(); pgtest.Stop(); os.Exit(code) }
