package security

import (
	"fmt"
	"strings"
)

// IAMPolicy represents a cloud provider IAM policy
type IAMPolicy struct {
	Provider string
	Role     string
	Actions  []string
	Resource string
}

// IAMValidator validates IAM policies follow least privilege principles
type IAMValidator struct {
	// Dangerous actions that should never be granted
	dangerousActions map[string][]string

	// Required actions for basic functionality
	requiredActions map[string][]string
}

// NewIAMValidator creates a new IAM validator
func NewIAMValidator() *IAMValidator {
	return &IAMValidator{
		dangerousActions: map[string][]string{
			"gcp": {
				"iam.serviceAccountKeys.create", // Can create persistent credentials
				"iam.serviceAccounts.setIamPolicy", // Can grant permissions
				"compute.projects.setCommonInstanceMetadata", // Can affect all VMs
				"resourcemanager.projects.setIamPolicy", // Can grant project-wide permissions
				"iam.roles.create", // Can create custom roles
				"iam.roles.update", // Can modify roles
				"compute.firewalls.delete", // Can remove security boundaries
				"compute.networks.delete", // Can remove network isolation
			},
			"aws": {
				"iam:CreateAccessKey", // Can create persistent credentials
				"iam:AttachUserPolicy", // Can grant permissions
				"iam:AttachRolePolicy", // Can grant permissions
				"iam:CreateRole", // Can create new roles
				"iam:UpdateRole", // Can modify roles
				"ec2:DeleteSecurityGroup", // Can remove security boundaries
				"ec2:DeleteVpc", // Can remove network isolation
			},
			"azure": {
				"Microsoft.Authorization/roleAssignments/write", // Can grant permissions
				"Microsoft.Authorization/roleDefinitions/write", // Can create roles
				"Microsoft.KeyVault/vaults/secrets/write", // Can create secrets
				"Microsoft.Network/networkSecurityGroups/delete", // Can remove security
			},
		},
		requiredActions: map[string][]string{
			"gcp": {
				"secretmanager.versions.access", // Read secrets
				"logging.logEntries.create", // Write logs
				"compute.instances.delete", // Self-termination
				"compute.instances.get", // Read own metadata
			},
			"aws": {
				"secretsmanager:GetSecretValue", // Read secrets
				"logs:CreateLogGroup", // Create log groups
				"logs:CreateLogStream", // Create log streams
				"logs:PutLogEvents", // Write logs
				"ec2:TerminateInstances", // Self-termination
			},
			"azure": {
				"Microsoft.KeyVault/vaults/secrets/read", // Read secrets
				"Microsoft.Insights/logs/write", // Write logs
				"Microsoft.Compute/virtualMachines/delete", // Self-termination
			},
		},
	}
}

// ValidatePolicy checks if an IAM policy follows security best practices
func (v *IAMValidator) ValidatePolicy(policy IAMPolicy) error {
	provider := strings.ToLower(policy.Provider)

	// Check for dangerous actions
	dangerous, exists := v.dangerousActions[provider]
	if exists {
		for _, action := range policy.Actions {
			for _, dangerousAction := range dangerous {
				if matchesAction(action, dangerousAction) {
					return fmt.Errorf("dangerous action detected: %s - this violates least privilege", action)
				}
			}
		}
	}

	// Check for wildcard permissions
	for _, action := range policy.Actions {
		if strings.Contains(action, "*") && !isAcceptableWildcard(action) {
			return fmt.Errorf("wildcard permission detected: %s - too broad", action)
		}
	}

	// Check resource scope
	if policy.Resource == "*" || policy.Resource == "" {
		return fmt.Errorf("resource scope too broad: should be limited to specific resources")
	}

	return nil
}

// CheckRequiredPermissions verifies all required permissions are present
func (v *IAMValidator) CheckRequiredPermissions(provider string, grantedActions []string) error {
	provider = strings.ToLower(provider)
	required, exists := v.requiredActions[provider]
	if !exists {
		return fmt.Errorf("unknown provider: %s", provider)
	}

	missing := []string{}
	for _, reqAction := range required {
		found := false
		for _, granted := range grantedActions {
			if matchesAction(granted, reqAction) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, reqAction)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required permissions: %v", missing)
	}

	return nil
}

// matchesAction checks if a granted action matches a required action
func matchesAction(granted, required string) bool {
	// Exact match
	if granted == required {
		return true
	}

	// Wildcard match (e.g., "compute.*" matches "compute.instances.delete")
	if strings.HasSuffix(granted, ".*") || strings.HasSuffix(granted, ":*") {
		prefix := strings.TrimSuffix(granted, "*")
		return strings.HasPrefix(required, prefix)
	}

	return false
}

// isAcceptableWildcard checks if a wildcard permission is acceptable
func isAcceptableWildcard(action string) bool {
	// Some wildcards are acceptable if scoped properly
	acceptablePatterns := []string{
		"logs:*", // Logging permissions are generally safe
		"monitoring:*", // Monitoring permissions are generally safe
		"cloudwatch:*", // CloudWatch permissions are generally safe
	}

	for _, pattern := range acceptablePatterns {
		if strings.HasPrefix(action, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}

	return false
}

// RecommendedGCPRoles returns the recommended minimal GCP roles
func RecommendedGCPRoles() []string {
	return []string{
		"roles/secretmanager.secretAccessor", // Read secrets only
		"roles/logging.logWriter", // Write logs only
		"roles/compute.instanceAdmin.v1", // Manage own instance (for self-termination)
	}
}

// RecommendedAWSPolicies returns the recommended minimal AWS policies
func RecommendedAWSPolicies() []string {
	return []string{
		"SecretsManagerReadOnly", // Read secrets only
		"CloudWatchLogsFullAccess", // Write logs (consider custom policy for write-only)
		// Custom policy needed for self-termination
	}
}

// RecommendedAzureRoles returns the recommended minimal Azure roles
func RecommendedAzureRoles() []string {
	return []string{
		"Key Vault Secrets User", // Read secrets only
		"Log Analytics Contributor", // Write logs
		// Custom role needed for self-termination
	}
}