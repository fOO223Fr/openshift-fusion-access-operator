/*
Copyright 2025.

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

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("FileSystemClaim Webhook", func() {
	var (
		validator  *FileSystemClaimValidator
		ctx        context.Context
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create a fake client with the scheme
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		validator = &FileSystemClaimValidator{Client: fakeClient}
	})

	Describe("ValidateCreate", func() {
		It("should allow creation with valid devices", func() {
			fsc := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, fsc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should allow creation with invalid devices (validation happens in controller)", func() {
			fsc := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n100"},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, fsc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should allow creation with empty devices", func() {
			fsc := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, fsc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})

	Describe("ValidateUpdate - LocalDiskCreated=False (Allow Updates)", func() {
		It("should allow device update when no status conditions exist", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n100"},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
			}

			// Create the FSC without conditions
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should allow device update when DeviceValidated=False", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n100"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "DeviceValidated",
							Status: metav1.ConditionFalse,
							Reason: "DeviceValidationFailed",
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
			}

			// Create the FSC with DeviceValidated=False
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should allow device update when LocalDiskCreated=False", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n200"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "DeviceValidated",
							Status: metav1.ConditionFalse,
							Reason: "DeviceValidationFailed",
						},
						{
							Type:   "LocalDiskCreated",
							Status: metav1.ConditionFalse,
							Reason: "LocalDiskCreationInProgress",
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"},
				},
			}

			// Create the FSC
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})

	Describe("ValidateUpdate - LocalDiskCreated=True (Block Updates)", func() {
		It("should reject device value change when LocalDiskCreated=True", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "LocalDiskCreated",
							Status:             metav1.ConditionTrue,
							Reason:             "LocalDiskCreationSucceeded",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme500n500"},
				},
			}

			// Create the FSC with LocalDiskCreated=True
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.devices cannot be modified"))
			Expect(err.Error()).To(ContainSubstring("LocalDisks were created"))
			Expect(warnings).To(BeNil())
		})

		It("should reject device order change when LocalDiskCreated=True", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "LocalDiskCreated",
							Status:             metav1.ConditionTrue,
							Reason:             "LocalDiskCreationSucceeded",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme2n2", "/dev/nvme1n1"},
				},
			}

			// Create the FSC with LocalDiskCreated=True
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.devices cannot be modified"))
			Expect(warnings).To(BeNil())
		})

		It("should reject adding device when LocalDiskCreated=True", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "LocalDiskCreated",
							Status:             metav1.ConditionTrue,
							Reason:             "LocalDiskCreationSucceeded",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"},
				},
			}

			// Create the FSC with LocalDiskCreated=True
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.devices cannot be modified"))
			Expect(warnings).To(BeNil())
		})

		It("should reject removing device when LocalDiskCreated=True", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "LocalDiskCreated",
							Status:             metav1.ConditionTrue,
							Reason:             "LocalDiskCreationSucceeded",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
			}

			// Create the FSC with LocalDiskCreated=True
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.devices cannot be modified"))
			Expect(warnings).To(BeNil())
		})

		It("should allow update to other fields when LocalDiskCreated=True", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
					Labels: map[string]string{
						"test": "old",
					},
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "LocalDiskCreated",
							Status:             metav1.ConditionTrue,
							Reason:             "LocalDiskCreationSucceeded",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
					Labels: map[string]string{
						"test": "new",
					},
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"}, // Same devices
				},
			}

			// Create the FSC with LocalDiskCreated=True
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})

	Describe("ValidateUpdate - Edge Cases", func() {
		It("should allow update when devices are identical (no change)", func() {
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"},
				},
				Status: FileSystemClaimStatus{
					Conditions: []metav1.Condition{
						{
							Type:               "LocalDiskCreated",
							Status:             metav1.ConditionTrue,
							Reason:             "LocalDiskCreationSucceeded",
							LastTransitionTime: metav1.Now(),
						},
					},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1", "/dev/nvme2n2"}, // Same devices
				},
			}

			// Create the FSC
			Expect(fakeClient.Create(ctx, oldFSC)).To(Succeed())

			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("should handle missing FSC gracefully (allow update)", func() {
			// Don't create the FSC - simulate it doesn't exist in the API
			oldFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nonexistent-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
			}

			newFSC := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nonexistent-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme2n2"},
				},
			}

			// Should allow because we can't fetch status
			warnings, err := validator.ValidateUpdate(ctx, oldFSC, newFSC)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})

	Describe("ValidateDelete", func() {
		It("should allow deletion", func() {
			fsc := &FileSystemClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fsc",
					Namespace: "ibm-spectrum-scale",
				},
				Spec: FileSystemClaimSpec{
					Devices: []string{"/dev/nvme1n1"},
				},
			}

			warnings, err := validator.ValidateDelete(ctx, fsc)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})
})
