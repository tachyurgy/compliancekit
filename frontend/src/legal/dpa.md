# Levelbrook Data Processing Addendum

**Effective: May 9, 2026**

This addendum sits alongside our Terms of Service and explains how Levelbrook handles personal data on your behalf. It applies whenever you upload information about a child, parent, or staff member.

## 1. Roles

You are the **data controller** — you decide what to upload and why. Levelbrook is the **data processor** — we handle the data on your instructions and only as needed to provide the service. We don't make decisions about your families' records on our own.

## 2. What kind of data we process

The data you upload typically includes:

- Names, addresses, phone numbers, and emails of children, parents, guardians, and staff.
- Birth dates and ages.
- Immunization records, health forms, and exemption letters.
- Background-check results and certifications for staff.
- License documents, incident reports, and inspection paperwork.
- Photos of paper documents you upload through the app or that parents and staff send through magic-link portals.

Some of this is sensitive (health information, background checks). We treat it accordingly.

## 3. How long we keep it

We keep your data for as long as your account is active. If you cancel, we keep it for 90 days so you can change your mind, then we delete it. You can request earlier deletion at any time by emailing us. We log every deletion.

We retain backups for 30 days after the source records are deleted. After that, every copy is gone.

## 4. Where we store it

In the United States, on Amazon Web Services, primarily in the us-east-1 region (Northern Virginia). We don't transfer data outside the United States.

## 5. Who has access

Inside Levelbrook, only the engineers who need access to keep the service running can read your data, and only when needed to fix a real problem. We log every access. We don't browse your data for fun, and we don't share it with anyone outside the subprocessors listed below.

Subprocessors:

- **AWS** (us-east-1): document storage, transactional email (SES), and database backups.
- **Stripe**: payment processing. Card numbers never touch our servers.
- **Mistral and Google**: OCR (turning uploaded images into searchable text). Documents are sent over TLS, processed, and not retained by them.
- **Twilio**: SMS reminders, only if you've turned on SMS for that recipient.
- **DigitalOcean**: API server hosting.

The current list lives at **levelbrook.com/subprocessors**. We'll email you 30 days before adding a new one.

## 6. Security

We use TLS 1.2+ on every connection. Documents in S3 are encrypted at rest with AES-256. We use bcrypt for password hashes. Database backups are encrypted. Two-factor authentication is on the roadmap. We run regular penetration tests and patch promptly.

Every authenticated action writes an audit log row with who did what, when, and from where. You can read your facility's audit log from Settings.

## 7. If we have a breach

If we discover a security incident that may have exposed your data, we'll email you within 72 hours of confirming it, share what we know, and tell you what we're doing to fix it. We will not bury bad news.

## 8. Your rights, on behalf of your families

You can ask us to:

- Export every piece of data we hold for your facility (also self-serve from Settings).
- Delete a specific record (one child, one staff member, one document).
- Delete your entire facility's data.
- Provide a copy of the audit log for any record.

We respond to these requests within 30 days. There's no charge for reasonable requests.

## 9. Our sub-processors' obligations

Each subprocessor is contractually required to handle data at least as carefully as we are. If one of them changes how they operate in a way that materially affects you, we'll let you know.

## 10. Disagreements

If you disagree with how we're handling data, email us at **levelbrookteam@gmail.com** first. We respond within two business days. If we can't work it out, you can cancel without penalty and take your data with you.

## 11. Changes

If we change this addendum, we'll email you 30 days before the new version takes effect and ask you to re-sign.
