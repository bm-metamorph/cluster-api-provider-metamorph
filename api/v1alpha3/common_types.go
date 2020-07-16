/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha3

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// Host is the hostname on which the API server is serving.
	Host string `json:"host"`

	// Port is the port on which the API server is serving.
	Port int `json:"port"`
}

// Image contains the details of the image to bootstrap the node.
type Image struct {
	// URL is a location of the image.
	URL string `json:"url"`
	// Checksum is a location of md5sum for the image.
	Checksum string `json:"checksum"`
}

// IPMIDetails contains the IPMI details of target node.
type IPMIDetails struct {
	Address                        string `json:"address"`
	CredentialsName                string `json:"credentialsName"`
	DisableCertificateVerification bool   `json:"disableCertificateVerification,omitempty"`
}
