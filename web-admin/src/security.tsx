import { useState } from "react";
import type { FormEvent } from "react";
import { api, errorText } from "./api";
import { Field, Notice } from "./sections";

type Message = { kind: "error" | "ok"; text: string } | null;

type Props = {
  onPasswordChanged: (message: string) => void;
};

export function Security({ onPasswordChanged }: Props) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<Message>(null);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setMessage(null);
    if (newPassword !== confirmPassword) {
      setMessage({ kind: "error", text: "Новый пароль и подтверждение не совпадают" });
      return;
    }
    setBusy(true);
    const res = await api("/auth/password", {
      method: "POST",
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    });
    setBusy(false);
    if (!res.ok) {
      setMessage({ kind: "error", text: errorText(res.body, "не удалось изменить пароль") });
      return;
    }
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    onPasswordChanged("Пароль изменён. Все сеансы завершены — войдите с новым паролем.");
  }

  return (
    <section>
      <h2>Безопасность</h2>
      <p className="hint">Смена пароля текущей учётной записи. После сохранения все активные сеансы будут завершены.</p>
      <Notice message={message} />
      <form className="card" onSubmit={submit}>
        <h3>Сменить пароль</h3>
        <Field label="Текущий пароль">
          <input
            type="password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>
        <Field label="Новый пароль">
          <input
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            minLength={5}
            required
          />
        </Field>
        <Field label="Повторите новый пароль">
          <input
            type="password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            autoComplete="new-password"
            minLength={5}
            required
          />
        </Field>
        <p className="hint">Не менее 5 символов, обязательно хотя бы одна буква и одна цифра.</p>
        <div className="form-actions">
          <button type="submit" disabled={busy}>{busy ? "Сохранение…" : "Изменить пароль"}</button>
        </div>
      </form>
    </section>
  );
}
