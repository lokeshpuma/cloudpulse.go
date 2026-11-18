import React, { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Activity, AlertTriangle, Clock, ShieldOff } from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import { Button, inputClassName } from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";

const AuthShell: React.FC<{ title: string; subtitle: string; children: React.ReactNode }> = ({
  title,
  subtitle,
  children,
}) => (
  <div className="min-h-screen flex items-center justify-center bg-slate-100 dark:bg-navy-950 p-4">
    <div className="w-full max-w-md rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-7 shadow-xl">
      <div className="flex items-center gap-2.5 mb-6">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand-600 text-white">
          <Activity className="h-6 w-6" aria-hidden="true" />
        </div>
        <div>
          <span className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">CloudPulse</span>
          <p className="text-xs text-slate-500 dark:text-slate-400">Infrastructure Monitoring Platform</p>
        </div>
      </div>
      <h1 className="text-xl font-semibold text-slate-900 dark:text-white">{title}</h1>
      <p className="mt-1 text-sm text-slate-500 dark:text-slate-400 mb-6">{subtitle}</p>
      {children}
    </div>
  </div>
);

const loginSchema = z.object({
  email: z.string().email("Enter a valid email address"),
  password: z.string().min(1, "Password is required"),
  remember_me: z.boolean().default(true),
});
type LoginForm = z.infer<typeof loginSchema>;

export const LoginPage: React.FC = () => {
  const { applyAuthPayload, enterDemoMode } = useAuth();
  const { notify } = useToast();
  const navigate = useNavigate();
  const [serverError, setServerError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
    defaultValues: { remember_me: true },
  });

  const onSubmit = async (values: LoginForm) => {
    setServerError(null);
    try {
      const payload = await api.login(values);
      applyAuthPayload(payload);
      notify({ type: "success", title: `Welcome back, ${payload.user.name}` });
      navigate(payload.organization.onboarding_completed ? "/" : "/onboarding");
    } catch (err) {
      setServerError(err instanceof Error ? err.message : "Sign in failed");
    }
  };

  return (
    <AuthShell title="Sign in to CloudPulse" subtitle="Monitor your servers, services, and alerts in real time.">
      {serverError && (
        <div role="alert" className="mb-4 rounded-lg border border-rose-200 bg-rose-50 dark:bg-rose-950/40 dark:border-rose-900 px-3.5 py-2.5 text-xs text-rose-700 dark:text-rose-300">
          {serverError}
        </div>
      )}
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
        <div>
          <label htmlFor="login-email" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
            Email address
          </label>
          <input id="login-email" type="email" autoComplete="email" className={inputClassName} {...register("email")} />
          {errors.email && <p className="mt-1 text-xs text-rose-600">{errors.email.message}</p>}
        </div>

        <div>
          <div className="flex items-center justify-between mb-1">
            <label htmlFor="login-password" className="text-xs font-medium text-slate-700 dark:text-slate-300">
              Password
            </label>
            <Link to="/forgot-password" className="text-xs font-medium text-brand-600 hover:underline">
              Forgot password?
            </Link>
          </div>
          <input id="login-password" type="password" autoComplete="current-password" className={inputClassName} {...register("password")} />
          {errors.password && <p className="mt-1 text-xs text-rose-600">{errors.password.message}</p>}
        </div>

        <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-400">
          <input type="checkbox" className="rounded border-slate-300 text-brand-600" {...register("remember_me")} />
          Remember me on this browser
        </label>

        <Button type="submit" className="w-full" size="lg" disabled={isSubmitting}>
          {isSubmitting ? "Signing in..." : "Sign in"}
        </Button>
      </form>

      <div className="mt-5 pt-5 border-t border-slate-200 dark:border-slate-800 space-y-3 text-center">
        <p className="text-xs text-slate-600 dark:text-slate-400">
          New to CloudPulse?{" "}
          <Link to="/register" className="font-medium text-brand-600 hover:underline">
            Create an organization
          </Link>
        </p>
        <Button
          type="button"
          variant="secondary"
          className="w-full"
          onClick={() => {
            enterDemoMode();
            navigate("/");
          }}
        >
          Explore Demo Mode (Sample Fixture Data)
        </Button>
      </div>
    </AuthShell>
  );
};

const registerSchema = z
  .object({
    name: z.string().min(2, "Full name must be at least 2 characters"),
    email: z.string().email("Enter a valid email address"),
    organization_name: z.string().min(2, "Organization name is required"),
    password: z
      .string()
      .min(8, "Password must be at least 8 characters")
      .regex(/[A-Z]/, "Must include an uppercase letter")
      .regex(/[a-z]/, "Must include a lowercase letter")
      .regex(/[0-9]/, "Must include a digit"),
    confirm_password: z.string(),
  })
  .refine((d) => d.password === d.confirm_password, {
    message: "Passwords do not match",
    path: ["confirm_password"],
  });
type RegisterForm = z.infer<typeof registerSchema>;

export const RegisterPage: React.FC = () => {
  const { applyAuthPayload } = useAuth();
  const { notify } = useToast();
  const navigate = useNavigate();
  const [serverError, setServerError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterForm>({
    resolver: zodResolver(registerSchema),
  });

  const onSubmit = async (values: RegisterForm) => {
    setServerError(null);
    try {
      const payload = await api.register({
        name: values.name,
        email: values.email,
        password: values.password,
        organization_name: values.organization_name,
      });
      applyAuthPayload(payload);
      notify({ type: "success", title: "Organization created", description: "Let's configure your first server." });
      navigate("/onboarding");
    } catch (err) {
      setServerError(err instanceof Error ? err.message : "Registration failed");
    }
  };

  return (
    <AuthShell title="Create your CloudPulse account" subtitle="Register an administrator account and organization.">
      {serverError && (
        <div role="alert" className="mb-4 rounded-lg border border-rose-200 bg-rose-50 dark:bg-rose-950/40 dark:border-rose-900 px-3.5 py-2.5 text-xs text-rose-700 dark:text-rose-300">
          {serverError}
        </div>
      )}
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-3.5" noValidate>
        <div>
          <label htmlFor="reg-name" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
            Full name
          </label>
          <input id="reg-name" className={inputClassName} {...register("name")} />
          {errors.name && <p className="mt-1 text-xs text-rose-600">{errors.name.message}</p>}
        </div>
        <div>
          <label htmlFor="reg-email" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
            Email address
          </label>
          <input id="reg-email" type="email" className={inputClassName} {...register("email")} />
          {errors.email && <p className="mt-1 text-xs text-rose-600">{errors.email.message}</p>}
        </div>
        <div>
          <label htmlFor="reg-org" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
            Organization name
          </label>
          <input id="reg-org" className={inputClassName} {...register("organization_name")} />
          {errors.organization_name && <p className="mt-1 text-xs text-rose-600">{errors.organization_name.message}</p>}
        </div>
        <div>
          <label htmlFor="reg-pw" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
            Password
          </label>
          <input id="reg-pw" type="password" className={inputClassName} {...register("password")} />
          {errors.password && <p className="mt-1 text-xs text-rose-600">{errors.password.message}</p>}
        </div>
        <div>
          <label htmlFor="reg-cpw" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
            Confirm password
          </label>
          <input id="reg-cpw" type="password" className={inputClassName} {...register("confirm_password")} />
          {errors.confirm_password && <p className="mt-1 text-xs text-rose-600">{errors.confirm_password.message}</p>}
        </div>

        <Button type="submit" className="w-full" size="lg" disabled={isSubmitting}>
          {isSubmitting ? "Creating account..." : "Create account"}
        </Button>
      </form>
      <p className="mt-5 text-center text-xs text-slate-600 dark:text-slate-400">
        Already have an account?{" "}
        <Link to="/login" className="font-medium text-brand-600 hover:underline">
          Sign in
        </Link>
      </p>
    </AuthShell>
  );
};

export const ForgotPasswordPage: React.FC = () => {
  const [email, setEmail] = useState("");
  const [result, setResult] = useState<{ instructions: string; dev_reset_token?: string } | null>(null);
  const [loading, setLoading] = useState(false);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      const res = await api.forgotPassword(email);
      setResult(res);
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell title="Reset your password" subtitle="Enter your account email to request a password reset token.">
      {result ? (
        <div className="space-y-4">
          <div className="rounded-lg border border-emerald-200 bg-emerald-50 dark:bg-emerald-950/30 dark:border-emerald-900 p-3.5 text-xs text-emerald-800 dark:text-emerald-200">
            {result.instructions}
          </div>
          {result.dev_reset_token && (
            <div className="rounded-lg border border-amber-200 bg-amber-50 dark:bg-amber-950/30 p-3 text-xs">
              <p className="font-semibold text-amber-900 dark:text-amber-200">Development Reset Token:</p>
              <code className="block mt-1 break-all font-mono">{result.dev_reset_token}</code>
              <Link
                to={`/reset-password?token=${encodeURIComponent(result.dev_reset_token)}`}
                className="inline-block mt-2 font-medium text-brand-600 hover:underline"
              >
                Continue to Reset Password →
              </Link>
            </div>
          )}
          <Link to="/login" className="block text-center text-xs font-medium text-brand-600 hover:underline">
            Back to Sign in
          </Link>
        </div>
      ) : (
        <form onSubmit={onSubmit} className="space-y-4">
          <div>
            <label htmlFor="fp-email" className="block text-xs font-medium mb-1">
              Email address
            </label>
            <input
              id="fp-email"
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className={inputClassName}
            />
          </div>
          <Button type="submit" className="w-full" disabled={loading}>
            {loading ? "Submitting..." : "Send Reset Instructions"}
          </Button>
          <p className="text-center text-xs">
            <Link to="/login" className="text-brand-600 hover:underline">
              Back to Sign in
            </Link>
          </p>
        </form>
      )}
    </AuthShell>
  );
};

export const ResetPasswordPage: React.FC = () => {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { notify } = useToast();
  const [token, setToken] = useState(params.get("token") ?? "");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await api.resetPassword(token, password);
      notify({ type: "success", title: "Password updated", description: "Please sign in with your new password." });
      navigate("/login");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reset failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthShell title="Set a new password" subtitle="Choose a strong password (min 8 chars, upper, lower, digit).">
      {error && (
        <div role="alert" className="mb-4 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {error}
        </div>
      )}
      <form onSubmit={onSubmit} className="space-y-4">
        <div>
          <label htmlFor="rp-token" className="block text-xs font-medium mb-1">
            Reset token
          </label>
          <input id="rp-token" required value={token} onChange={(e) => setToken(e.target.value)} className={inputClassName} />
        </div>
        <div>
          <label htmlFor="rp-pw" className="block text-xs font-medium mb-1">
            New password
          </label>
          <input id="rp-pw" type="password" required value={password} onChange={(e) => setPassword(e.target.value)} className={inputClassName} />
        </div>
        <Button type="submit" className="w-full" disabled={loading}>
          {loading ? "Updating..." : "Update Password"}
        </Button>
      </form>
    </AuthShell>
  );
};

export const UnauthorizedPage: React.FC = () => (
  <AuthShell title="Access Denied" subtitle="Your current role does not have permission to view this resource.">
    <div className="flex flex-col items-center text-center space-y-4">
      <ShieldOff className="h-12 w-12 text-rose-500" aria-hidden="true" />
      <p className="text-xs text-slate-600 dark:text-slate-400">
        Contact an organization administrator if you believe you require elevated access.
      </p>
      <Link to="/">
        <Button>Return to Dashboard</Button>
      </Link>
    </div>
  </AuthShell>
);

export const SessionExpiredPage: React.FC = () => (
  <AuthShell title="Session Expired" subtitle="For your security, your CloudPulse session has expired.">
    <div className="flex flex-col items-center text-center space-y-4">
      <div className="flex items-center gap-2 text-amber-600">
        <Clock className="h-10 w-10" aria-hidden="true" />
        <AlertTriangle className="h-6 w-6" aria-hidden="true" />
      </div>
      <p className="text-xs text-slate-600 dark:text-slate-400">
        Please sign in again to continue monitoring your infrastructure.
      </p>
      <Link to="/login">
        <Button>Sign In Again</Button>
      </Link>
    </div>
  </AuthShell>
);
