package upgrade_kyma

import (
	"fmt"
	"time"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/cls"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process/provisioning"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/storage"
	"github.com/sirupsen/logrus"
)

type clsUpgradeProvisionStep struct {
	config           *cls.Config
	instanceProvider provisioning.ClsProvisioner
	operationManager *process.UpgradeKymaOperationManager
}

func NewClsUpgradeProvisionStep(config *cls.Config, ip provisioning.ClsProvisioner, repo storage.Operations) *clsUpgradeProvisionStep {
	return &clsUpgradeProvisionStep{
		config:           config,
		operationManager: process.NewUpgradeKymaOperationManager(repo),
		instanceProvider: ip,
	}
}

func (s *clsUpgradeProvisionStep) Name() string {
	return "CLS_UpgradeProvision"
}

func (s *clsUpgradeProvisionStep) Run(operation internal.UpgradeKymaOperation, log logrus.FieldLogger) (internal.UpgradeKymaOperation, time.Duration, error) {
	if operation.Cls.Instance.InstanceID != "" {
		log.Infof("CLS instance already exists")
		return operation, 0, nil
	}

	globalAccountID := operation.ProvisioningParameters.ErsContext.GlobalAccountID

	skrRegion := operation.ProvisioningParameters.Parameters.Region
	smRegion, err := cls.DetermineServiceManagerRegion(skrRegion)
	if err != nil {
		failureReason := fmt.Sprintf("Unable to determine CLS Service Manager in region %s", *skrRegion)
		log.Errorf("%s: %v", failureReason, err)
		return s.operationManager.OperationFailed(operation, failureReason, log)
	}

	smCredentials, err := cls.FindCredentials(s.config.ServiceManager, smRegion)
	if err != nil {
		failureReason := fmt.Sprintf("Unable to find credentials for CLS Service Manager in region %s", operation.Cls.Region)
		log.Errorf("%s: %v", failureReason, err)
		return s.operationManager.OperationFailed(operation, failureReason, log)
	}

	log.Infof("Starting provisioning a CLS instance for global account %s", globalAccountID)

	smClient := operation.SMClientFactory.ForCredentials(smCredentials)
	skrInstanceID := operation.InstanceID
	result, err := s.instanceProvider.Provision(smClient, &cls.ProvisionRequest{
		GlobalAccountID: globalAccountID,
		Region:          smRegion,
		SKRInstanceID:   skrInstanceID,
		Instance:        operation.Cls.Instance.InstanceKey(),
	})
	if err != nil {
		failureReason := fmt.Sprintf("Unable to provision a CLS instance for global account %s", globalAccountID)
		log.Errorf("%s: %v", failureReason, err)
		return s.operationManager.OperationFailed(operation, failureReason, log)
	}

	log.Infof("Finished provisioning a CLS instance for global account %s", globalAccountID)

	op, repeat := s.operationManager.UpdateOperation(operation, func(operation *internal.UpgradeKymaOperation) {
		operation.Cls.Region = result.Region
		operation.Cls.Instance.InstanceID = result.InstanceID
		operation.Cls.Instance.ProvisioningTriggered = result.ProvisioningTriggered
	}, log)
	if repeat != 0 {
		log.Errorf("Unable to update operation")
		return operation, time.Second, nil
	}

	return op, 0, nil
}