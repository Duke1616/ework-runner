package dao

import "gorm.io/gorm"

func InitTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&Task{},
		&TaskParamOverrideRule{},
		&TaskRunParamOverride{},
		&TaskExecutionNotificationRule{},
		&CodebookProject{},
		&Codebook{},
		&CodebookVersion{},
		&Runner{},
		&Variable{},
		&ExecutionPool{},
		&ExecutionPoolBinding{},
		&TaskExecution{},
		&TaskExecutionLog{},
		&ExecutionCancellation{},
		&ArtifactRelease{},
		&ProjectSource{},
		&AIConversation{},
		&AIMessage{},
		&AIChangeSet{},
	)
}
