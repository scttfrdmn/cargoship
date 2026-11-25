package encryption

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// TestEncryptManifest_RecordsKeyARNAndDecryptOmitsAlias pins #912: the manifest
// records the key KMS used, not the alias passed in, and Decrypt names a key only
// when it is a specific key, so a re-pointed alias can't break decryption.
func TestEncryptManifest_RecordsKeyARNAndDecryptOmitsAlias(t *testing.T) {
	const arn = "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab"
	var decryptKeyIDs []*string
	m := &MockKMSClient{
		generateDataKeyFunc: func(_ context.Context, in *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
			key := make([]byte, 32)
			return &kms.GenerateDataKeyOutput{Plaintext: key, CiphertextBlob: append([]byte("encrypted:"), key...), KeyId: aws.String(arn)}, nil
		},
	}
	m.decryptFunc = func(_ context.Context, in *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
		decryptKeyIDs = append(decryptKeyIDs, in.KeyId)
		return &kms.DecryptOutput{Plaintext: in.CiphertextBlob[10:]}, nil
	}
	id := ManifestIdentity{UploadID: "u"}

	sealed, err := EncryptManifestBytes(context.Background(), m, "alias/archive", []byte(`{}`), id)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.KMSKeyID != arn {
		t.Fatalf("KMSKeyID = %q, want the key ARN KMS returned", sealed.KMSKeyID)
	}

	if _, err := DecryptManifestBytes(context.Background(), m, sealed, id); err != nil {
		t.Fatal(err)
	}
	sealed.KMSKeyID = "alias/archive" // a pre-#912 record
	if _, err := DecryptManifestBytes(context.Background(), m, sealed, id); err != nil {
		t.Fatal(err)
	}
	if len(decryptKeyIDs) != 2 || aws.ToString(decryptKeyIDs[0]) != arn || decryptKeyIDs[1] != nil {
		t.Errorf("Decrypt KeyIds = %v, want the ARN, then none for an alias", decryptKeyIDs)
	}

	// A KMS response without KeyId falls back to the requested key.
	m.generateDataKeyFunc = nil
	sealed, err = EncryptManifestBytes(context.Background(), m, arn, []byte(`{}`), id)
	if err != nil || sealed.KMSKeyID != arn {
		t.Errorf("KMSKeyID = %q (err %v), want %q", sealed.KMSKeyID, err, arn)
	}
}

func TestIsSpecificKeyID(t *testing.T) {
	for id, want := range map[string]bool{
		"": false, "alias/x": false, "arn:aws:kms:us-east-1:1:alias/x": false,
		"arn:aws:kms:us-east-1:1:key/abc": true, "1234abcd-12ab-34cd-56ef-1234567890ab": true,
	} {
		if got := isSpecificKeyID(id); got != want {
			t.Errorf("isSpecificKeyID(%q) = %v, want %v", id, got, want)
		}
	}
}
