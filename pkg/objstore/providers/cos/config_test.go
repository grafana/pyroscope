package cos

import "testing"

func TestConfig_Validate(t *testing.T) {
	type fields struct {
		Bucket    string
		Region    string
		AppID     string
		Endpoint  string
		SecretKey string
		SecretID  string
		HTTP      HTTPConfig
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "ok endpoint",
			fields: fields{
				Endpoint:  "http://bucket-123.cos.ap-beijing.myqcloud.com",
				SecretID:  "sid",
				SecretKey: "skey",
			},
			wantErr: false,
		},
		{
			name: "ok bucket-AppID-region",
			fields: fields{
				Bucket:    "bucket",
				AppID:     "123",
				Region:    "ap-beijing",
				SecretID:  "sid",
				SecretKey: "skey",
			},
			wantErr: false,
		},
		{
			name: "missing skey",
			fields: fields{
				Bucket: "bucket",
				AppID:  "123",
				Region: "ap-beijing",
			},
			wantErr: true,
		},
		{
			name: "missing bucket",
			fields: fields{
				AppID:     "123",
				Region:    "ap-beijing",
				SecretID:  "sid",
				SecretKey: "skey",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Bucket:    tt.fields.Bucket,
				Region:    tt.fields.Region,
				AppID:     tt.fields.AppID,
				Endpoint:  tt.fields.Endpoint,
				SecretKey: tt.fields.SecretKey,
				SecretID:  tt.fields.SecretID,
				HTTP:      tt.fields.HTTP,
			}
			if err := c.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
