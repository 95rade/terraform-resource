package terraform

import (
	"errors"
	"fmt"
	"terraform-resource/models"
	"terraform-resource/storage"
)

type Action struct {
	Client          Client
	StateFile       StateFile
	DeleteOnFailure bool
}

type Result struct {
	Version storage.Version
	Output  map[string]interface{}
}

func (a Action) Apply() (Result, error) {
	err := a.setup()
	if err != nil {
		return Result{}, err
	}

	result, err := a.attemptApply()
	if err != nil {
		err = fmt.Errorf("Apply Error: %s", err)
	}

	alreadyDeleted := false
	if err != nil && a.DeleteOnFailure {
		_, destroyErr := a.attemptDestroy()
		if destroyErr != nil {
			err = fmt.Errorf("%s\nDestroy Error: %s", err, destroyErr)
		} else {
			alreadyDeleted = true
		}
	}

	if err != nil && alreadyDeleted == false {
		uploadErr := a.uploadTaintedStatefile()
		if uploadErr != nil {
			err = fmt.Errorf("Destroy Error: %s\nUpload Error: %s", err, uploadErr)
		}
	}

	return result, err
}

func (a Action) attemptApply() (Result, error) {
	if err := a.Client.Apply(); err != nil {
		return Result{}, fmt.Errorf("Failed to run terraform apply.\nError: %s", err)
	}

	storageVersion, err := a.StateFile.Upload()
	if err != nil {
		return Result{}, fmt.Errorf("Failed to upload state file: %s", err)
	}

	clientOutput, err := a.Client.Output()
	if err != nil {
		return Result{}, fmt.Errorf("Failed to terraform output.\nError: %s", err)
	}
	return Result{
		Output:  clientOutput,
		Version: storageVersion,
	}, nil
}

func (a Action) Destroy() (Result, error) {
	err := a.setup()
	if err != nil {
		return Result{}, err
	}

	result, err := a.attemptDestroy()

	if err != nil {
		uploadErr := a.uploadTaintedStatefile()
		if uploadErr != nil {
			err = fmt.Errorf("Destroy Error: %s\nUpload Error: %s", err, uploadErr)
		}
	}

	return result, err
}

func (a Action) attemptDestroy() (Result, error) {
	if err := a.Client.Destroy(); err != nil {
		return Result{}, fmt.Errorf("Failed to run terraform destroy.\nError: %s", err)
	}

	storageVersion, err := a.StateFile.Delete()
	if err != nil {
		return Result{}, fmt.Errorf("Failed to delete state file: %s", err)
	}
	return Result{
		Output:  map[string]interface{}{},
		Version: storageVersion,
	}, nil
}

func (a *Action) setup() error {
	stateFileExists, err := a.StateFile.Exists()
	if err != nil {
		return err
	}

	if stateFileExists == false {
		stateFileExists, err = a.StateFile.ExistsAsTainted()
		if err != nil {
			return err
		}
		if stateFileExists {
			a.StateFile = a.StateFile.ConvertToTainted()
		}
	}

	if stateFileExists {
		_, err = a.StateFile.Download()
		if err != nil {
			return err
		}
		outputs, err := a.Client.Output()
		if err != nil {
			return err
		}
		a.Client.Model = models.Terraform{Vars: outputs}.Merge(a.Client.Model)
	}
	return nil
}

func (a *Action) uploadTaintedStatefile() error {
	errMsg := ""
	_, deleteErr := a.StateFile.Delete()
	if deleteErr != nil {
		errMsg = fmt.Sprintf("Delete original state file error: %s", deleteErr)
	}
	a.StateFile = a.StateFile.ConvertToTainted()

	_, uploadErr := a.StateFile.Upload()
	if uploadErr != nil {
		errMsg = fmt.Sprintf("%s\nUpload Error: %s", errMsg, uploadErr)
	}

	if len(errMsg) > 0 {
		return errors.New(errMsg)
	}
	return nil
}