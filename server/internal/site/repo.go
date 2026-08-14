package site

import "gorm.io/gorm"

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// LoadAll returns all settings as a map. If the table is empty, defaults
// are seeded first so the system is usable immediately after migration.
func (r *Repo) LoadAll() (map[string]string, error) {
	var rows []Setting
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		defaults := Defaults()
		for k, v := range defaults {
			if err := r.db.Create(&Setting{Key: k, Value: v}).Error; err != nil {
				return nil, err
			}
		}
		return defaults, nil
	}
	m := make(map[string]string, len(rows))
	for _, row := range rows {
		m[row.Key] = row.Value
	}
	return m, nil
}

// Set upserts a single setting row.
func (r *Repo) Set(key, value string) error {
	return r.db.Save(&Setting{Key: key, Value: value}).Error
}
