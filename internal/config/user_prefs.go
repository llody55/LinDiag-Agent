package config

import (
"encoding/json"
"os"
)

type UserPreferences struct {
AutoConfirmLowRisk    bool `json:"auto_confirm_low_risk"`
AutoConfirmMediumRisk bool `json:"auto_confirm_medium_risk"`
}

var userPrefs = &UserPreferences{}

func LoadUserPreferences() {
data, err := os.ReadFile("user_prefs.json")
if err != nil {
return
}
_ = json.Unmarshal(data, userPrefs)
}

func SaveUserPreferences() {
data, _ := json.Marshal(userPrefs)
_ = os.WriteFile("user_prefs.json", data, 0644)
}

func GetUserPreferences() *UserPreferences {
return userPrefs
}

func SetAutoConfirmLowRisk(enabled bool) {
userPrefs.AutoConfirmLowRisk = enabled
SaveUserPreferences()
}

func SetAutoConfirmMediumRisk(enabled bool) {
userPrefs.AutoConfirmMediumRisk = enabled
SaveUserPreferences()
}
