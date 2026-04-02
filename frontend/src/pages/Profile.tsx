import { useState, useEffect } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Topbar } from '../components/Topbar'
import { useAuth } from '../context/AuthContext'
import { totp as totpApi, users, mfaSettings } from '../api/client'
import type { TOTPSetupResponse } from '../api/types'
import './Settings.css'

export function Profile() {
  const { user, refreshUser, mfaEnrollmentRequired, pwPolicyNoncompliant, clearPwPolicyNoncompliant } = useAuth()

  // Global MFA requirement
  const [mfaRequired, setMfaRequired] = useState(false)
  useEffect(() => {
    mfaSettings.get().then(r => setMfaRequired(r.required)).catch(() => {})
  }, [])

  // ── Change Password ──────────────────────────────────────────────────────────
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [pwMsg, setPwMsg] = useState('')
  const [pwSaving, setPwSaving] = useState(false)

  const handleChangePassword = async () => {
    if (!currentPw || !newPw) { setPwMsg('Both fields are required.'); return }
    setPwSaving(true)
    setPwMsg('')
    try {
      await users.changePassword({ current_password: currentPw, new_password: newPw })
      setCurrentPw('')
      setNewPw('')
      setPwMsg('Password updated.')
      clearPwPolicyNoncompliant()
    } catch (e: unknown) {
      setPwMsg(e instanceof Error ? e.message : 'Failed to update password')
    } finally {
      setPwSaving(false)
    }
  }

  // ── TOTP Enrollment ──────────────────────────────────────────────────────────
  const totpEnabled = user?.totp_enabled ?? false

  const [setup, setSetup] = useState<TOTPSetupResponse | null>(null)
  const [confirmCode, setConfirmCode] = useState('')
  const [confirmMsg, setConfirmMsg] = useState('')
  const [confirmSaving, setConfirmSaving] = useState(false)
  const [setupLoading, setSetupLoading] = useState(false)

  // Disable own TOTP
  const [disableCode, setDisableCode] = useState('')
  const [disableMsg, setDisableMsg] = useState('')
  const [disableSaving, setDisableSaving] = useState(false)
  const [showDisable, setShowDisable] = useState(false)

  // Reset setup state when user TOTP status changes.
  useEffect(() => {
    if (totpEnabled) setSetup(null)
  }, [totpEnabled])

  const handleStartSetup = async () => {
    setSetupLoading(true)
    setConfirmMsg('')
    try {
      const res = await totpApi.setup()
      setSetup(res)
    } catch (e: unknown) {
      setConfirmMsg(e instanceof Error ? e.message : 'Failed to generate secret')
    } finally {
      setSetupLoading(false)
    }
  }

  const handleConfirm = async () => {
    if (!confirmCode || confirmCode.length !== 6) {
      setConfirmMsg('Enter the 6-digit code from your app.')
      return
    }
    setConfirmSaving(true)
    setConfirmMsg('')
    try {
      await totpApi.confirm(confirmCode)
      setConfirmCode('')
      setSetup(null)
      setConfirmMsg('')
      await refreshUser()
    } catch (e: unknown) {
      setConfirmMsg(e instanceof Error ? e.message : 'Invalid code')
    } finally {
      setConfirmSaving(false)
    }
  }

  const handleDisableOwn = async () => {
    if (!disableCode || disableCode.length !== 6) {
      setDisableMsg('Enter your current 6-digit TOTP code to confirm.')
      return
    }
    setDisableSaving(true)
    setDisableMsg('')
    try {
      await totpApi.disableOwn(disableCode)
      setDisableCode('')
      setShowDisable(false)
      await refreshUser()
    } catch (e: unknown) {
      setDisableMsg(e instanceof Error ? e.message : 'Invalid code')
    } finally {
      setDisableSaving(false)
    }
  }

  return (
    <>
      <Topbar title="Profile" />
      <div className="content">
        <div className="tab-content">

          {/* Password policy warning banner */}
          {pwPolicyNoncompliant && (
            <div style={{
              background: 'rgba(239,68,68,0.1)',
              border: '1px solid var(--red)',
              borderRadius: 8,
              padding: '12px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              fontSize: '0.875rem',
              color: 'var(--red)',
            }}>
              <span style={{ fontSize: '1.1rem' }}>⚠</span>
              <span>
                Your password no longer meets the current security policy. Please update it below.
              </span>
            </div>
          )}

          {/* MFA enrollment warning banner */}
          {mfaEnrollmentRequired && (
            <div style={{
              background: 'var(--amber-dim, rgba(245,158,11,0.12))',
              border: '1px solid var(--amber, #f59e0b)',
              borderRadius: 8,
              padding: '12px 16px',
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              fontSize: '0.875rem',
              color: 'var(--amber, #f59e0b)',
            }}>
              <span style={{ fontSize: '1.1rem' }}>⚠</span>
              <span>
                Multi-factor authentication is required for your account. Set up TOTP below before your next login — your grace login has been used.
              </span>
            </div>
          )}

          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Account</span>
            </div>
            <div className="settings-field-row">
              <label className="settings-label">Email</label>
              <span style={{ fontSize: '0.875rem', color: 'var(--text2)' }}>{user?.email ?? '—'}</span>
            </div>
            <div className="settings-field-row">
              <label className="settings-label">Role</label>
              <span className="app-pill-type">{user?.role ?? '—'}</span>
            </div>
          </section>

          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Change Password</span>
            </div>
            <div className="settings-field-row">
              <label className="settings-label">Current password</label>
              <input
                className="settings-input"
                type="password"
                placeholder="••••••••"
                value={currentPw}
                onChange={e => setCurrentPw(e.target.value)}
              />
            </div>
            <div className="settings-field-row">
              <label className="settings-label">New password</label>
              <input
                className="settings-input"
                type="password"
                placeholder="••••••••"
                value={newPw}
                onChange={e => setNewPw(e.target.value)}
              />
            </div>
            <div className="settings-actions">
              <button className="settings-btn primary" onClick={handleChangePassword} disabled={pwSaving}>
                {pwSaving ? 'Saving…' : 'Update Password'}
              </button>
              {pwMsg && <span className="settings-status-msg">{pwMsg}</span>}
            </div>
          </section>

          <section className="settings-section">
            <div className="section-header">
              <span className="section-title">Two-Factor Authentication</span>
              {totpEnabled && (
                <span className="totp-enabled-badge">Enabled</span>
              )}
            </div>

            {totpEnabled ? (
              <>
                <p className="settings-placeholder" style={{ marginBottom: 12 }}>
                  TOTP is active on your account. Your authenticator app generates codes required at login.
                </p>
                {mfaRequired && (
                  <p className="settings-placeholder" style={{ marginBottom: 12, color: 'var(--text3)', fontSize: '0.8rem' }}>
                    MFA is required globally — disabling TOTP is not permitted.
                  </p>
                )}
                {!mfaRequired && !showDisable && (
                  <button
                    className="settings-btn danger settings-btn--sm"
                    onClick={() => { setShowDisable(true); setDisableMsg('') }}
                  >
                    Disable TOTP
                  </button>
                )}
                {!mfaRequired && showDisable && (
                  <div>
                    <p className="settings-placeholder" style={{ marginBottom: 8, color: 'var(--red)' }}>
                      Enter your current authenticator code to disable TOTP.
                    </p>
                    <div className="settings-field-row">
                      <label className="settings-label">Code</label>
                      <input
                        className="settings-input"
                        type="text"
                        inputMode="numeric"
                        maxLength={6}
                        placeholder="000000"
                        value={disableCode}
                        onChange={e => setDisableCode(e.target.value.replace(/\D/g, ''))}
                        style={{ maxWidth: 120, fontFamily: 'var(--mono)', letterSpacing: '0.15em' }}
                      />
                    </div>
                    <div className="settings-actions">
                      <button
                        className="settings-btn danger"
                        onClick={handleDisableOwn}
                        disabled={disableSaving || disableCode.length !== 6}
                      >
                        {disableSaving ? 'Disabling…' : 'Confirm Disable'}
                      </button>
                      <button className="settings-btn secondary" onClick={() => { setShowDisable(false); setDisableCode(''); setDisableMsg('') }}>
                        Cancel
                      </button>
                      {disableMsg && <span className="settings-status-msg" style={{ color: 'var(--red)' }}>{disableMsg}</span>}
                    </div>
                  </div>
                )}
              </>
            ) : setup ? (
              <>
                <p className="settings-placeholder" style={{ marginBottom: 16 }}>
                  Scan the QR code with your authenticator app (Google Authenticator, Authy, etc.), then enter the 6-digit code to confirm.
                </p>
                <div className="totp-qr-container">
                  <QRCodeSVG value={setup.uri} size={180} bgColor="#ffffff" fgColor="#000000" />
                </div>
                <p className="settings-placeholder" style={{ marginTop: 8, marginBottom: 16, fontSize: '0.75rem' }}>
                  Can't scan? Enter this key manually: <code style={{ userSelect: 'all', letterSpacing: '0.1em' }}>{setup.secret}</code>
                </p>
                <div className="settings-field-row">
                  <label className="settings-label">Confirm code</label>
                  <input
                    className="settings-input"
                    type="text"
                    inputMode="numeric"
                    maxLength={6}
                    placeholder="000000"
                    value={confirmCode}
                    onChange={e => setConfirmCode(e.target.value.replace(/\D/g, ''))}
                    style={{ maxWidth: 120, fontFamily: 'var(--mono)', letterSpacing: '0.15em' }}
                    autoFocus
                  />
                </div>
                <div className="settings-actions">
                  <button
                    className="settings-btn primary"
                    onClick={handleConfirm}
                    disabled={confirmSaving || confirmCode.length !== 6}
                  >
                    {confirmSaving ? 'Verifying…' : 'Activate TOTP'}
                  </button>
                  <button className="settings-btn secondary" onClick={() => { setSetup(null); setConfirmCode(''); setConfirmMsg('') }}>
                    Cancel
                  </button>
                  {confirmMsg && <span className="settings-status-msg" style={{ color: 'var(--red)' }}>{confirmMsg}</span>}
                </div>
              </>
            ) : (
              <>
                <p className="settings-placeholder" style={{ marginBottom: 12 }}>
                  Add an extra layer of security. Once enabled, you'll need a code from your authenticator app at each login.
                </p>
                <button
                  className="settings-btn primary"
                  onClick={handleStartSetup}
                  disabled={setupLoading}
                >
                  {setupLoading ? 'Generating…' : 'Set Up TOTP'}
                </button>
                {confirmMsg && <span className="settings-status-msg" style={{ color: 'var(--red)', marginLeft: 12 }}>{confirmMsg}</span>}
              </>
            )}
          </section>

        </div>
      </div>
    </>
  )
}
