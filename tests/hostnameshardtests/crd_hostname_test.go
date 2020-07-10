/*
 * Copyright 2019-2020 VMware, Inc.
 * All Rights Reserved.
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*   http://www.apache.org/licenses/LICENSE-2.0
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

package hostnameshardtests

import (
	akov1alpha1 "ako/pkg/apis/ako/v1alpha1"
	"ako/pkg/cache"
	avinodes "ako/pkg/nodes"
	"ako/pkg/objects"
	"ako/tests/integrationtest"
	"testing"
	"time"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeHostRule struct {
	name                  string
	namespace             string
	fqdn                  string
	sslKeyCertificate     string
	networkSecurityPolicy string
	wafPolicy             string
	applicationProfile    string
	httpPolicySets        []string
}

func (hr FakeHostRule) HostRule() *akov1alpha1.HostRule {
	hostrule := &akov1alpha1.HostRule{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: hr.namespace,
			Name:      hr.name,
		},
		Spec: akov1alpha1.HostRuleSpec{
			VirtualHost: akov1alpha1.HostRuleVirtualHost{
				Fqdn: hr.fqdn,
				TLS: akov1alpha1.HostRuleTLS{
					SSLKeyCertificate: akov1alpha1.HostRuleSecret{
						Name: hr.sslKeyCertificate,
						Type: "ref",
					},
					Termination: "edge",
				},
				HTTPPolicy: akov1alpha1.HostRuleHTTPPolicy{
					PolicySets: hr.httpPolicySets,
					Overwrite:  false,
				},
				NetworkSecurityPolicy: hr.networkSecurityPolicy,
				WAFPolicy:             hr.wafPolicy,
				ApplicationProfile:    hr.applicationProfile,
			},
		},
	}

	return hostrule
}

func SetupHostRule(t *testing.T, hrname, fqdn string, secure bool) {
	hostrule := FakeHostRule{
		name:                  hrname,
		namespace:             "default",
		fqdn:                  fqdn,
		networkSecurityPolicy: "thisisahostruleref-netsec",
		wafPolicy:             "thisisahostruleref-waf",
		applicationProfile:    "thisisahostruleref-appprof",
		httpPolicySets:        []string{"thisisahostruleref-httpps-1"},
	}
	if secure {
		hostrule.sslKeyCertificate = "thisisahostruleref-sslkey"
	}

	hrCreate := hostrule.HostRule()
	if _, err := CRDClient.AkoV1alpha1().HostRules("default").Create(hrCreate); err != nil {
		t.Fatalf("error in adding HostRule: %v", err)
	}
}

func TeardownHostRule(t *testing.T, hrname string) {
	if err := CRDClient.AkoV1alpha1().HostRules("default").Delete(hrname, nil); err != nil {
		t.Fatalf("error in deleting HostRule: %v", err)
	}
}

func VerifyActiveHostRule(g *gomega.WithT, vsKey cache.NamespaceName, hrnsname string) {
	mcache := cache.SharedAviObjCache()
	g.Eventually(func() bool {
		sniCache, found := mcache.VsCacheMeta.AviCacheGet(vsKey)
		sniCacheObj, ok := sniCache.(*cache.AviVsCache)
		if ok && found &&
			sniCacheObj.ServiceMetadataObj.CRDStatus.Value == hrnsname &&
			sniCacheObj.ServiceMetadataObj.CRDStatus.Status == "ACTIVE" {
			return true
		}
		return false
	}, 20*time.Second).Should(gomega.Equal(true))
}

func TestHostnameCreateHostRule(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"
	SetUpIngressForCacheSyncCheck(t, modelName, true, true)

	SetupHostRule(t, hrname, "foo.com", true)

	g.Eventually(func() string {
		hostrule, _ := CRDClient.AkoV1alpha1().HostRules("default").Get(hrname, metav1.GetOptions{})
		return hostrule.Status.Status
	}, 10*time.Second).Should(gomega.Equal("Accepted"))

	g.Eventually(func() string {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		return nodes[0].SniNodes[0].SSLKeyCertAviRef
	}, 10*time.Second).Should(gomega.ContainSubstring("thisisahostruleref"))
	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].SniNodes[0].WafPolicyRef).To(gomega.ContainSubstring("thisisahostruleref-waf"))
	g.Expect(nodes[0].SniNodes[0].NsPolicyRef).To(gomega.ContainSubstring("thisisahostruleref-netsec"))
	g.Expect(nodes[0].SniNodes[0].AppProfileRef).To(gomega.ContainSubstring("thisisahostruleref-appprof"))
	g.Expect(nodes[0].SniNodes[0].HttpPolicySetRefs).To(gomega.HaveLen(1))
	g.Expect(nodes[0].SniNodes[0].HttpPolicySetRefs[0]).To(gomega.ContainSubstring("thisisahostruleref-httpps"))

	sniVSKey := cache.NamespaceName{Namespace: "admin", Name: "cluster--foo.com"}
	VerifyActiveHostRule(g, sniVSKey, "default/samplehr-foo")

	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameCreateHostRuleBeforeIngress(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"
	SetupHostRule(t, hrname, "foo.com", true)

	g.Eventually(func() string {
		hostrule, _ := CRDClient.AkoV1alpha1().HostRules("default").Get(hrname, metav1.GetOptions{})
		return hostrule.Status.Status
	}, 10*time.Second).Should(gomega.Equal("Accepted"))

	SetUpIngressForCacheSyncCheck(t, modelName, true, true)

	g.Eventually(func() string {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		if len(nodes[0].SniNodes) == 1 {
			return nodes[0].SniNodes[0].SSLKeyCertAviRef
		}
		return ""
	}, 10*time.Second).Should(gomega.ContainSubstring("thisisahostruleref-sslkey"))

	TeardownHostRule(t, hrname)

	g.Eventually(func() string {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		if len(nodes[0].SniNodes) == 1 {
			return nodes[0].SniNodes[0].SSLKeyCertAviRef
		}
		return ""
	}, 10*time.Second).Should(gomega.Equal(""))
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameInsecureToSecureHostRule(t *testing.T) {
	// insecure ingress to secure VS via Hostrule
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"
	SetUpIngressForCacheSyncCheck(t, modelName, false, false)

	mcache := cache.SharedAviObjCache()
	vsKey := cache.NamespaceName{Namespace: "admin", Name: "cluster--Shared-L7-0"}
	g.Eventually(func() int {
		vsCache, _ := mcache.VsCacheMeta.AviCacheGet(vsKey)
		vsCacheObj, _ := vsCache.(*cache.AviVsCache)
		return len(vsCacheObj.SNIChildCollection)
	}, 15*time.Second).Should(gomega.Equal(0))

	SetupHostRule(t, hrname, "foo.com", true)

	g.Eventually(func() int {
		vsCache, _ := mcache.VsCacheMeta.AviCacheGet(vsKey)
		vsCacheObj, _ := vsCache.(*cache.AviVsCache)
		return len(vsCacheObj.SNIChildCollection)
	}, 15*time.Second).Should(gomega.Equal(1))

	sniVSKey := cache.NamespaceName{Namespace: "admin", Name: "cluster--foo.com"}
	VerifyActiveHostRule(g, sniVSKey, "default/samplehr-foo")

	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].SniNodes[0].SSLKeyCertAviRef).To(gomega.ContainSubstring("thisisahostruleref-sslkey"))
	g.Expect(nodes[0].SniNodes[0].WafPolicyRef).To(gomega.ContainSubstring("thisisahostruleref-waf"))
	g.Expect(nodes[0].SniNodes[0].NsPolicyRef).To(gomega.ContainSubstring("thisisahostruleref-netsec"))

	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameMultiIngressToSecureHostRule(t *testing.T) {
	// 1 insecure ingress, 1 secure ingress -> secure VS via Hostrule
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"

	// creating secure default/foo.com/foo
	SetUpIngressForCacheSyncCheck(t, modelName, true, true)

	// creating insecure red/foo.com/bar
	ingressObject := integrationtest.FakeIngress{
		Name:        "foo-with-targets-2",
		Namespace:   "red",
		DnsNames:    []string{"foo.com"},
		Paths:       []string{"/bar"},
		ServiceName: "avisvc",
	}
	ingrFake := ingressObject.Ingress()
	if _, err := KubeClient.ExtensionsV1beta1().Ingresses("red").Create(ingrFake); err != nil {
		t.Fatalf("error in updating Ingress: %v", err)
	}

	SetupHostRule(t, hrname, "foo.com", true)

	g.Eventually(func() int {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		if len(nodes[0].SniNodes) > 0 {
			return len(nodes[0].SniNodes[0].PoolGroupRefs)
		}
		return 0
	}, 20*time.Second).Should(gomega.Equal(2))
	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].PoolRefs).To(gomega.HaveLen(0))
	g.Expect(nodes[0].SniNodes[0].PoolRefs).To(gomega.HaveLen(2))
	g.Expect(nodes[0].SniNodes[0].SSLKeyCertAviRef).To(gomega.ContainSubstring("thisisahostruleref-sslkey"))
	g.Expect(nodes[0].SniNodes[0].SSLKeyCertRefs).To(gomega.HaveLen(0))

	sniVSKey := cache.NamespaceName{Namespace: "admin", Name: "cluster--foo.com"}
	VerifyActiveHostRule(g, sniVSKey, "default/samplehr-foo")

	if err := KubeClient.ExtensionsV1beta1().Ingresses("red").Delete("foo-with-targets-2", nil); err != nil {
		t.Fatalf("Couldn't DELETE the Ingress %v", err)
	}
	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameMultiIngressSwitchHostRuleFqdn(t *testing.T) {
	// 2 insecure ingresses -> VS via Hostrule
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"

	// creating insecure default/foo.com/foo
	SetUpIngressForCacheSyncCheck(t, modelName, false, false)

	// creating insecure red/voo.com/voo
	ingressObject := integrationtest.FakeIngress{
		Name:        "voo-with-targets",
		Namespace:   "red",
		DnsNames:    []string{"voo.com"},
		Paths:       []string{"/voo"},
		ServiceName: "avisvc",
	}
	ingrFake := ingressObject.Ingress()
	if _, err := KubeClient.ExtensionsV1beta1().Ingresses("red").Create(ingrFake); err != nil {
		t.Fatalf("error in updating Ingress: %v", err)
	}

	// hostrule for foo.com
	SetupHostRule(t, hrname, "foo.com", true)

	// voo.com must be insecure, foo.com must be secure
	// both foo.com and voo.com fall in the SAME shard
	g.Eventually(func() bool {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		if len(nodes[0].SniNodes) == 1 &&
			len(nodes[0].PoolRefs) == 1 {
			return true
		}
		return false
	}, 30*time.Second).Should(gomega.Equal(true))
	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].PoolRefs[0].Name).To(gomega.Equal("cluster--voo.com_voo-red-voo-with-targets"))
	g.Expect(nodes[0].SniNodes[0].Name).To(gomega.Equal("cluster--foo.com"))
	g.Expect(nodes[0].SniNodes[0].PoolRefs[0].Name).To(gomega.Equal("cluster--default-foo.com_foo-foo-with-targets"))

	// change hostrule for foo.com to voo.com
	hrUpdate := FakeHostRule{
		name:              hrname,
		namespace:         "default",
		fqdn:              "voo.com",
		sslKeyCertificate: "thisisahostruleref-sslkey",
	}.HostRule()
	hrUpdate.ResourceVersion = "2"
	if _, err := CRDClient.AkoV1alpha1().HostRules("default").Update(hrUpdate); err != nil {
		t.Fatalf("error in updating HostRule: %v", err)
	}

	// foo.com would be insecure, voo.com would become secure now
	g.Eventually(func() bool {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		if len(nodes[0].SniNodes) == 1 &&
			nodes[0].SniNodes[0].Name == "cluster--voo.com" &&
			len(nodes[0].PoolRefs) == 1 {
			return true
		}
		return false
	}, 30*time.Second).Should(gomega.Equal(true))
	_, aviModel = objects.SharedAviGraphLister().Get(modelName)
	nodes = aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].PoolRefs[0].Name).To(gomega.Equal("cluster--foo.com_foo-default-foo-with-targets"))
	g.Expect(nodes[0].SniNodes[0].PoolRefs[0].Name).To(gomega.Equal("cluster--red-voo.com_voo-voo-with-targets"))

	if err := KubeClient.ExtensionsV1beta1().Ingresses("red").Delete("voo-with-targets", nil); err != nil {
		t.Fatalf("Couldn't Delete the Ingress %v", err)
	}
	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameGoodToBadHostRule(t *testing.T) {
	// create secure ingress, apply good hostrule, transition to bad
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"
	SetUpIngressForCacheSyncCheck(t, modelName, false, false)
	SetupHostRule(t, hrname, "foo.com", true)

	sniVSKey := cache.NamespaceName{Namespace: "admin", Name: "cluster--foo.com"}
	VerifyActiveHostRule(g, sniVSKey, "default/samplehr-foo")

	// update hostrule with bad ref
	hrUpdate := FakeHostRule{
		name:               hrname,
		namespace:          "default",
		fqdn:               "voo.com",
		wafPolicy:          "BADREF",
		applicationProfile: "thisisahostruleref-appprof",
	}.HostRule()
	hrUpdate.ResourceVersion = "2"
	if _, err := CRDClient.AkoV1alpha1().HostRules("default").Update(hrUpdate); err != nil {
		t.Fatalf("error in updating HostRule: %v", err)
	}

	g.Eventually(func() string {
		hostrule, _ := CRDClient.AkoV1alpha1().HostRules("default").Get(hrname, metav1.GetOptions{})
		return hostrule.Status.Status
	}, 10*time.Second).Should(gomega.Equal("Rejected"))

	// the last applied hostrule values would exist
	g.Eventually(func() string {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		return nodes[0].SniNodes[0].SSLKeyCertAviRef
	}, 10*time.Second).Should(gomega.ContainSubstring("thisisahostruleref"))
	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].SniNodes[0].WafPolicyRef).To(gomega.ContainSubstring("thisisahostruleref-waf"))
	g.Expect(nodes[0].SniNodes[0].NsPolicyRef).To(gomega.ContainSubstring("thisisahostruleref-netsec"))
	g.Expect(nodes[0].SniNodes[0].AppProfileRef).To(gomega.ContainSubstring("thisisahostruleref-appprof"))

	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameInsecureHostAndHostrule(t *testing.T) {
	// create insecure ingress, insecure hostrule, nothing should be applied
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"
	SetUpIngressForCacheSyncCheck(t, modelName, false, false)
	SetupHostRule(t, hrname, "foo.com", false)

	g.Eventually(func() int {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		return len(nodes[0].PoolRefs)
	}, 10*time.Second).Should(gomega.Equal(1))
	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].SniNodes).To(gomega.HaveLen(0))

	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}

func TestHostnameValidToInvalidHostSwitch(t *testing.T) {
	// create insecure host foo.com, attach hostrule, change hostrule to non existing bar.com
	// foo.com should become insecure again
	// change hostrule back to foo.com and it should become secure again
	g := gomega.NewGomegaWithT(t)

	modelName := "admin/cluster--Shared-L7-0"
	hrname := "samplehr-foo"
	SetUpIngressForCacheSyncCheck(t, modelName, false, false)
	SetupHostRule(t, hrname, "foo.com", true)

	sniVSKey := cache.NamespaceName{Namespace: "admin", Name: "cluster--foo.com"}
	VerifyActiveHostRule(g, sniVSKey, "default/samplehr-foo")

	hrUpdate := FakeHostRule{
		name:              hrname,
		namespace:         "default",
		fqdn:              "bar.com",
		sslKeyCertificate: "thisisahostruleref-sslkey",
	}.HostRule()
	hrUpdate.ResourceVersion = "2"
	if _, err := CRDClient.AkoV1alpha1().HostRules("default").Update(hrUpdate); err != nil {
		t.Fatalf("error in updating HostRule: %v", err)
	}

	g.Eventually(func() int {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		return len(nodes[0].PoolRefs)
	}, 10*time.Second).Should(gomega.Equal(1))
	_, aviModel := objects.SharedAviGraphLister().Get(modelName)
	nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].PoolRefs[0].Name).To(gomega.Equal("cluster--foo.com_foo-default-foo-with-targets"))

	// change back to good host
	hrUpdate = FakeHostRule{
		name:              hrname,
		namespace:         "default",
		fqdn:              "foo.com",
		sslKeyCertificate: "thisisahostruleref-sslkey",
	}.HostRule()
	hrUpdate.ResourceVersion = "3"
	if _, err := CRDClient.AkoV1alpha1().HostRules("default").Update(hrUpdate); err != nil {
		t.Fatalf("error in updating HostRule: %v", err)
	}

	g.Eventually(func() int {
		_, aviModel := objects.SharedAviGraphLister().Get(modelName)
		nodes := aviModel.(*avinodes.AviObjectGraph).GetAviVS()
		return len(nodes[0].PoolRefs)
	}, 10*time.Second).Should(gomega.Equal(0))
	_, aviModel = objects.SharedAviGraphLister().Get(modelName)
	nodes = aviModel.(*avinodes.AviObjectGraph).GetAviVS()
	g.Expect(nodes[0].SniNodes[0].PoolRefs[0].Name).To(gomega.Equal("cluster--default-foo.com_foo-foo-with-targets"))

	TeardownHostRule(t, hrname)
	TearDownIngressForCacheSyncCheck(t, modelName)
}
