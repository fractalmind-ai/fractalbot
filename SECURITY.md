# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| Latest  | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly.

### How to Report

Please send an email to **elliottica937@gmail.com** with:

- A description of the vulnerability
- Steps to reproduce the issue
- Any potential impact or exploit scenario
- If applicable, a proposed fix or mitigation

### What to Expect

- We will acknowledge receipt of your report within 48 hours
- We will provide a detailed response within 7 days indicating the next steps
- You will receive credit for your discovery (unless you prefer to remain anonymous)

### Guidelines

- Please do not disclose security vulnerabilities publicly until they have been fixed
- Give us reasonable time to investigate and address the issue
- We appreciate your help in keeping this project secure!

## Security Best Practices

When using FractalBot, consider the following:

### Channel Security

- **Allowed Users**: Configure `allowedUsers` lists carefully to prevent unauthorized access
- **Token Management**: Never commit bot tokens; use environment variables
- **Websocket Security**: Ensure origin validation is enabled in production
- **Rate Limiting**: Consider implementing rate limiting for WebSocket connections

### Agent Permissions

- **Sandboxing**: Enable agent sandboxing for group/public sessions
- **Tool Whitelisting**: Only allow necessary tools in non-main sessions
- **File Access**: Configure proper file system permissions for agent workspaces
- **Command Execution**: Review and sanitize commands before execution

### Network Security

- **Bind Address**: Default to `127.0.0.1` (local-only) unless public access needed
- **TLS**: Enable TLS for WebSocket connections in production
- **Firewall**: Configure firewall rules to restrict port access
- **HTTPS**: Always use HTTPS for external channel connections (Telegram, Slack, Discord)

### Data Protection

- **Sensitive Data**: Never store passwords, API keys, or tokens in logs
- **Logs**: Be careful when sharing logs that might contain sensitive information
- **Workspace**: Keep workspace contents private if they contain sensitive information
- **Environment Variables**: Use environment variables for secrets, never commit them

## Dependency Security

This project uses Go's standard library and minimal dependencies:

- `nhooyr.io/websocket` - WebSocket library
- `gopkg.in/yaml.v3` - YAML configuration

We recommend:
- Keeping Go installation updated
- Running `go get -u ./...` regularly
- Reviewing dependency updates for security fixes
- Checking for CVEs in dependencies regularly

## Private Information

Never commit the following to the repository:

- Bot tokens (Telegram, Slack, Discord)
- API keys
- Passwords or credentials
- Personal information
- Private keys or certificates
- Agent credentials or access tokens

Use environment variables or secure configuration files for sensitive data.

## Hardening Checklist

Before deploying to production:

- [ ] WebSocket origin validation enabled
- [ ] TLS/SSL configured for external access
- [ ] Bot tokens stored in environment variables
- [ ] Allowed users lists configured
- [ ] Agent sandboxing enabled for non-main sessions
- [ ] Firewall rules configured for port access
- [ ] Logs are not exposing sensitive data
- [ ] File system permissions are restrictive
- [ ] HTTPS enforced for external connections

## Incident Response

If a security incident occurs:

1. Immediately disable affected systems
2. Assess the scope of the breach
3. Notify affected users (if applicable)
4. Patch vulnerabilities
5. Update documentation
6. Conduct post-incident review

## References

- [OWASP WebSocket Security](https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet)
- [Go Security Best Practices](https://go.dev/doc/security)
- [Telegram Bot Security](https://core.telegram.org/bots#security)
- [Slack Bot Security](https://api.slack.com/docs/security-best-practices)
- [Discord Bot Security](https://discord.com/developers/docs/security)
