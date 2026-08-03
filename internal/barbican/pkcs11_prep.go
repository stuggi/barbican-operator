package barbican

import (
	barbicanv1beta1 "github.com/openstack-k8s-operators/barbican-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PKCS11PrepJob func
func PKCS11PrepJob(instance *barbicanv1beta1.Barbican, labels map[string]string, annotations map[string]string) *batchv1.Job {
	// The PKCS11 Prep job just needs the main barbican config files, and the files
	// needed to communicate with the relevant HSM.
	pkcs11Volumes := []corev1.Volume{
		GetScriptVolume(instance.Name + "-scripts"),
	}
	pkcs11Volumes = append(pkcs11Volumes, GetVolumes(instance.Name)...)

	pkcs11Mounts := []corev1.VolumeMount{
		GetScriptVolumeMount(),
	}
	pkcs11Mounts = append(pkcs11Mounts, GetVolumeMounts()...)

	// add CA cert if defined
	if instance.Spec.BarbicanAPI.TLS.CaBundleSecretName != "" {
		pkcs11Volumes = append(pkcs11Volumes, instance.Spec.BarbicanAPI.TLS.CreateVolume())
		pkcs11Mounts = append(pkcs11Mounts, instance.Spec.BarbicanAPI.TLS.CreateVolumeMounts(nil)...)
	}

	// add any HSM volumes
	pkcs11Volumes = append(pkcs11Volumes, GetHSMVolumes(*instance.Spec.PKCS11)...)
	pkcs11Mounts = append(pkcs11Mounts, GetHSMVolumeMounts(instance.Spec.PKCS11.ClientDataPath)...)

	// This job runs as root (not a kolla artifact): the vendor HSM client
	// library setup performed by generate_pkcs11_keys.sh is not verified to
	// work under a non-root UID without real HSM hardware, so root access is
	// kept here as a documented exception -- see docs/remove-kolla-plan.md.
	runAsUser := int64(0)
	envVars := map[string]env.Setter{}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-pkcs11-prep",
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					ServiceAccountName: instance.RbacResourceName(),
					Containers: []corev1.Container{
						{
							Name: instance.Name + "-pkcs11-prep",
							Command: []string{
								ScriptMountPoint + "/generate_pkcs11_keys.sh",
							},
							Image: instance.Spec.BarbicanAPI.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          env.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: pkcs11Mounts,
						},
					},
				},
			},
		},
	}

	job.Spec.Template.Spec.Volumes = pkcs11Volumes

	return job
}
