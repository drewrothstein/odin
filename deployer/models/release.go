package models

import (
	"fmt"
	"time"

	"github.com/coinbase/odin/aws"
	"github.com/coinbase/step/aws/s3"
	"github.com/coinbase/step/utils/is"
	"github.com/coinbase/step/utils/to"
)

// ReleaseError error
type ReleaseError struct {
	Error *string
	Cause *string
}

// Release is the Data Structure passed between Client to Deployer
type Release struct {
	// Useful information from AWS
	AwsAccountID *string `json:"aws_account_id,omitempty"`
	AwsRegion    *string `json:"aws_region,omitempty"`

	UUID      *string `json:"uuid,omitempty"`       // Generated By server
	ReleaseID *string `json:"release_id,omitempty"` // Generated Client

	ProjectName *string `json:"project_name,omitempty"`
	ConfigName  *string `json:"config_name,omitempty"`
	Bucket      *string `json:"bucket,omitempty"` // Bucket with Additional Data in it

	Subnets []*string `json:"subnets,omitempty"`
	Timeout *int      `json:"timeout,omitempty"` // How long should we try and deploy in seconds

	CreatedAt     *time.Time `json:"created_at,omitempty"`
	releaseSHA256 string

	Success *bool `json:"success,omitempty"`

	Image *string `json:"ami,omitempty"`

	userdata       *string // Not serialized
	UserDataSHA256 *string `json:"user_data_sha256,omitempty"`
	UserDataKMSKey *string `json:"user_data_kms_key,omitempty"`

	// LifeCycleHooks
	LifeCycleHooks map[string]*LifeCycleHook `json:"lifecycle,omitempty"`

	// Maintain a Log to look at what has happened
	Healthy *bool `json:"healthy,omitempty"`

	// Where the previous Catch Error should be located
	Error *ReleaseError `json:"error,omitempty"`

	// AWS Service is Downloaded
	Services map[string]*Service `json:"services,omitempty"` // Downloaded From S3
}

//////////
// Getters
//////////

// rootPath to s3
func (release *Release) rootPath() string {
	return fmt.Sprintf("%v/%v/%v", *release.AwsAccountID, *release.ProjectName, *release.ConfigName)
}

// LockPath returns
func (release *Release) LockPath() *string {
	s := fmt.Sprintf("%v/lock", release.rootPath())
	return &s
}

// HaltPath returns
func (release *Release) HaltPath() *string {
	s := fmt.Sprintf("%v/halt", release.rootPath())
	return &s
}

// ReleasePath returns
func (release *Release) ReleasePath() *string {
	s := fmt.Sprintf("%v/%v/release", release.rootPath(), *release.ReleaseID)
	return &s
}

// UserDataPath returns
func (release *Release) UserDataPath() *string {
	s := fmt.Sprintf("%v/%v/userdata", release.rootPath(), *release.ReleaseID)
	return &s
}

func (release *Release) errorPrefix() string {
	if release.ReleaseID == nil {
		return fmt.Sprintf("Release Error:")
	}

	return fmt.Sprintf("Release(%v) Error:", *release.ReleaseID)
}

//////////
// Setters
//////////

// SetUUID returns
func (release *Release) SetUUID() {
	release.UUID = to.TimeUUID("release-")
}

// SetDefaultRegionAccount returns
func (release *Release) SetDefaultRegionAccount(region *string, account *string) {
	if is.EmptyStr(release.AwsAccountID) {
		release.AwsAccountID = account
	}

	if is.EmptyStr(release.AwsRegion) {
		release.AwsRegion = region
	}

	if is.EmptyStr(release.Bucket) && account != nil {
		release.Bucket = to.Strp(fmt.Sprintf("coinbase-odin-%v", *account))
	}
}

// SetDefaultsWithUserData sets the default values including userdata fetched from S3
func (release *Release) SetDefaultsWithUserData(s3c aws.S3API) error {
	release.SetDefaults()
	err := release.DownloadUserData(s3c)
	if err != nil {
		return err
	}

	for _, service := range release.Services {
		if service != nil {
			service.SetUserData(release.UserData())
		}
	}

	return nil
}

// SetDefaults assigns default values
func (release *Release) SetDefaults() {
	if release.Timeout == nil {
		release.Timeout = to.Intp(600) // Default to 10 minutes
	}

	if release.Healthy == nil {
		release.Healthy = to.Boolp(false)
	}

	release.SetDefaultKMSKey()

	for name, lc := range release.LifeCycleHooks {
		if lc != nil {
			lc.SetDefaults(release.AwsRegion, release.AwsAccountID, name)
		}
	}

	for name, service := range release.Services {
		if service != nil {
			service.SetDefaults(release, name)
		}
	}
}

// SetDefaultKMSKey sets the default KMS key to be used
func (release *Release) SetDefaultKMSKey() {
	if release.UserDataKMSKey == nil {
		// Default alias to the default s3 KMS key
		release.UserDataKMSKey = to.Strp("alias/aws/s3")
	}
}

//////////
// Validate
//////////

// Validate returns
func (release *Release) Validate(s3c aws.S3API) error {
	if err := release.ValidateAttributes(); err != nil {
		return fmt.Errorf("%v %v", release.errorPrefix(), err.Error())
	}

	if err := release.ValidateReleaseSHA(s3c); err != nil {
		return fmt.Errorf("%v %v", release.errorPrefix(), err.Error())
	}

	if err := release.ValidateUserDataSHA(s3c); err != nil {
		return fmt.Errorf("%v %v", release.errorPrefix(), err.Error())
	}

	if err := release.ValidateServices(); err != nil {
		return fmt.Errorf("%v %v", release.errorPrefix(), err.Error())
	}

	return nil
}

// ValidateAttributes validates attributes
func (release *Release) ValidateAttributes() error {
	if release == nil {
		// Extra paranoid
		return fmt.Errorf("Release is nil")
	}

	if is.EmptyStr(release.ProjectName) {
		return fmt.Errorf("ProjectName must be defined")
	}

	if is.EmptyStr(release.ConfigName) {
		return fmt.Errorf("ConfigName must be defined")
	}

	if is.EmptyStr(release.UUID) {
		return fmt.Errorf("UUID must be defined")
	}

	if is.EmptyStr(release.AwsRegion) {
		return fmt.Errorf("AwsRegion must be defined")
	}

	if is.EmptyStr(release.AwsAccountID) {
		return fmt.Errorf("AwsAccountID must be defined")
	}

	if is.EmptyStr(release.UserDataSHA256) {
		return fmt.Errorf("UserDataSHA256 must be defined")
	}

	if is.EmptyStr(release.ReleaseID) {
		return fmt.Errorf("ReleaseID must be defined")
	}

	if is.EmptyStr(release.Bucket) {
		return fmt.Errorf("Bucket must be defined")
	}

	if release.CreatedAt == nil {
		return fmt.Errorf("CreatedAt must be defined")
	}

	// Created at date must be after 5 mins ago, and before 2 mins from now (wiggle room)
	if !is.WithinTimeFrame(release.CreatedAt, 300*time.Second, 120*time.Second) {
		return fmt.Errorf("Created at older than 5 mins (or in the future)")
	}

	return nil
}

// ValidateReleaseSHA returns
func (release *Release) ValidateReleaseSHA(s3c aws.S3API) error {
	var s3Release Release
	err := s3.GetStruct(s3c, release.Bucket, release.ReleasePath(), &s3Release)
	if err != nil {
		return fmt.Errorf("Error Getting Release struct with %v", err.Error())
	}

	expected := to.SHA256Struct(s3Release)

	if expected != release.releaseSHA256 {
		return fmt.Errorf("Release SHA incorrect expected %v, got %v", expected, release.releaseSHA256)
	}

	return nil
}

// Validates the userdata has the correct SHA for the release
func (release *Release) ValidateUserDataSHA(s3c aws.S3API) error {
	err := release.DownloadUserData(s3c)

	if err != nil {
		return fmt.Errorf("Error Getting UserData with %v", err.Error())
	}

	userdataSha := to.SHA256Str(release.UserData())
	if userdataSha != *release.UserDataSHA256 {
		return fmt.Errorf("UserData SHA incorrect expected %v, got %v", userdataSha, *release.UserDataSHA256)
	}

	return nil
}

// UserData returns user data
func (release *Release) UserData() *string {
	return release.userdata
}

// DownloadUserData fetches and populates the User data from S3
func (release *Release) DownloadUserData(s3c aws.S3API) error {
	userdataBytes, err := s3.Get(s3c, release.Bucket, release.UserDataPath())

	if err != nil {
		return err
	}

	release.SetUserData(to.Strp(string(*userdataBytes)))
	return nil
}

// SetUserData sets the User data
func (release *Release) SetUserData(userdata *string) {
	release.userdata = userdata
}

// SetReleaseSHA256 sets the release SHA
func (release *Release) SetReleaseSHA256(sha string) {
	release.releaseSHA256 = sha
}

// ValidateServices returns
func (release *Release) ValidateServices() error {
	if release.Services == nil {
		return fmt.Errorf("Services nil")
	}

	if len(release.Services) == 0 {
		return fmt.Errorf("Services empty")
	}

	for name, service := range release.Services {
		if service == nil {
			return fmt.Errorf("Service %v is nil", name)
		}

		err := service.Validate()
		if err != nil {
			return err
		}
	}

	return nil
}
