# Deployment Guide

## Google App Engine Deployment

### Prerequisites
1. Google Cloud CLI installed and configured
2. Project setup in Google Cloud Console
3. App Engine API enabled

### Setup for First Time

1. **Copy app.yaml.example to app.yaml:**
   ```bash
   cp app.yaml.example app.yaml
   ```

2. **Edit app.yaml with your actual values:**
   - Replace all placeholder values with real credentials
   - Update FIREBASE_PROJECT_ID, SUPABASE_URL, etc.
   - Set production ALLOWED_ORIGINS

3. **Important Security Notes:**
   - `app.yaml` is in .gitignore and should NEVER be committed
   - Keep your credentials secure and rotate them regularly
   - Use different credentials for staging/production

### Deployment Commands

```bash
# Deploy to App Engine
gcloud app deploy app.yaml

# View logs
gcloud app logs tail -s default

# Open deployed app
gcloud app browse
```

### Environment Variables to Configure

#### Required for Production:
- `FIREBASE_PROJECT_ID`: Your Firebase project ID
- `FIREBASE_PRIVATE_KEY`: Firebase service account private key
- `FIREBASE_CLIENT_EMAIL`: Firebase service account email
- `SUPABASE_URL`: Your Supabase project URL
- `SUPABASE_SERVICE_ROLE_KEY`: Supabase service role key
- `JWT_SECRET`: Strong random string for JWT signing
- `ALLOWED_ORIGINS`: Your frontend domain(s)

#### Optional but Recommended:
- `GEMINI_API_KEY`: For AI features
- `REDIS_URL`: For caching
- `SMTP_*`: For email notifications

### Security Checklist

- [ ] All sensitive values replaced in app.yaml
- [ ] app.yaml added to .gitignore
- [ ] Production ALLOWED_ORIGINS set correctly
- [ ] Strong JWT_SECRET generated
- [ ] ENVIRONMENT set to "production"
- [ ] DEBUG set to "false"

### Troubleshooting

1. **Build Errors**: Check Go version compatibility
2. **Environment Variables**: Verify all required vars are set
3. **CORS Issues**: Check ALLOWED_ORIGINS configuration
4. **Database Connection**: Verify Supabase credentials
