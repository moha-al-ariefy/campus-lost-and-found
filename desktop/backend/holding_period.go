package backend

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
)

// An unset environment variable keeps the stored value. No duration is assumed for a new installation.
func LoadHoldingPeriod(db *sql.DB) error {
	value, configured := os.LookupEnv("CAMPUS_HOLDING_PERIOD_DAYS")
	if !configured {
		return nil
	}
	days, err := strconv.ParseInt(value, 10, 32)
	if err != nil || days <= 0 {
		return fmt.Errorf("CAMPUS_HOLDING_PERIOD_DAYS must be a positive whole number")
	}
	_, err = db.Exec(`INSERT INTO holding_period_settings(id,days) VALUES(1,?)
		ON CONFLICT(id) DO UPDATE SET days=excluded.days`, days)
	return err
}
