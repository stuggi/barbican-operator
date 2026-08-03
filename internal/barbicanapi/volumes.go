package barbicanapi

import (
	"fmt"
	"slices"

	barbicanv1beta1 "github.com/openstack-k8s-operators/barbican-operator/api/v1beta1"
	barbican "github.com/openstack-k8s-operators/barbican-operator/internal/barbican"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/storage"
	corev1 "k8s.io/api/core/v1"
)

// GetAPIVolumesAndMounts returns the volumes and mounts for a BarbicanAPI deployment.
// overwriteKeys lists the defaultConfigOverwrite filenames that need SubPath
// mounts into /etc/barbican/ (e.g. policy.yaml).
func GetAPIVolumesAndMounts(instance *barbicanv1beta1.BarbicanAPI, overwriteKeys []string) ([]corev1.Volume, []corev1.VolumeMount, error) {
	apiVolumes := []corev1.Volume{
		barbican.GetCustomConfigVolume(instance.Name),
		barbican.GetLogVolume(),
		barbican.GetRunHttpdVolume(),
		barbican.GetVarLogHttpdVolume(),
	}

	apiVolumeMounts := []corev1.VolumeMount{
		barbican.GetCustomConfigVolumeMount(),
		barbican.GetLogVolumeMount(),
	}
	apiVolumeMounts = append(apiVolumeMounts, barbican.GetAPIHttpdVolumeMounts()...)
	apiVolumeMounts = append(apiVolumeMounts, barbican.GetConfigOverwriteVolumeMounts(overwriteKeys)...)

	// prepend general config volumes and mounts
	apiVolumes = append(barbican.GetVolumes("barbican"), apiVolumes...)
	apiVolumeMounts = append(barbican.GetVolumeMounts(), apiVolumeMounts...)

	// add CA cert if defined
	if instance.Spec.TLS.CaBundleSecretName != "" {
		apiVolumes = append(apiVolumes, instance.Spec.TLS.CreateVolume())
		apiVolumeMounts = append(apiVolumeMounts, instance.Spec.TLS.CreateVolumeMounts(nil)...)
	}

	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		if instance.Spec.TLS.API.Enabled(endpt) {
			var tlsEndptCfg tls.GenericService
			switch endpt {
			case service.EndpointPublic:
				tlsEndptCfg = instance.Spec.TLS.API.Public
			case service.EndpointInternal:
				tlsEndptCfg = instance.Spec.TLS.API.Internal
			}

			svc, err := tlsEndptCfg.ToService()
			if err != nil {
				return nil, nil, err
			}
			// 10-barbican_wsgi_main.conf's SSLCertificateFile/SSLCertificateKeyFile
			// point at these final paths (see barbican_controller.go's
			// httpdVhostConfig) -- without this override CreateVolumeMounts
			// falls back to the old kolla staging path and httpd fails to
			// find its TLS cert/key.
			certMount := fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			keyMount := fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
			svc.CertMount = &certMount
			svc.KeyMount = &keyMount
			apiVolumes = append(apiVolumes, svc.CreateVolume(endpt.String()))
			apiVolumeMounts = append(apiVolumeMounts, svc.CreateVolumeMounts(endpt.String())...)
		}
	}

	// Add PKCS11 volumes
	if slices.Contains(instance.Spec.EnabledSecretStores, barbicanv1beta1.SecretStorePKCS11) && instance.Spec.PKCS11 != nil {
		apiVolumes = append(apiVolumes, barbican.GetHSMVolumes(*instance.Spec.PKCS11)...)
		apiVolumeMounts = append(apiVolumeMounts, barbican.GetHSMVolumeMounts(instance.Spec.PKCS11.ClientDataPath)...)
	}

	// ExtraMounts
	extraVols, extraMounts := barbican.GetExtraVolumes(
		instance.Spec.ExtraMounts,
		[]storage.PropagationType{barbican.BarbicanAPI, barbican.Barbican},
	)
	apiVolumes = append(apiVolumes, extraVols...)
	apiVolumeMounts = append(apiVolumeMounts, extraMounts...)

	return apiVolumes, apiVolumeMounts, nil
}
