package services

import "testing"

func TestParseCRMLeadAssessment(t *testing.T) {
	got, err := parseCRMLeadAssessment("```json\n{\"stage\":\"warm\",\"confidence\":0.86,\"reason\":\"Menanyakan harga produk\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != "warm" || got.Confidence != 0.86 || got.Reason != "Menanyakan harga produk" {
		t.Fatalf("hasil klasifikasi tidak sesuai: %#v", got)
	}
}

func TestParseCRMLeadAssessmentRejectsCustomer(t *testing.T) {
	if _, err := parseCRMLeadAssessment(`{"stage":"customer","confidence":1,"reason":"sudah membeli"}`); err == nil {
		t.Fatal("AI tidak boleh menetapkan status customer")
	}
}
