// Deprecated: Firebase auth is handled directly via useAuthStore.signInWithGoogle()
// Kept for backward compatibility
export function useLogin() {
  return { mutate: () => {}, isPending: false, error: null };
}
