import { useEffect, useState } from "react";

/** Returns `value` delayed by `ms` — useful for damping search input churn. */
export function useDebounced<T>(value: T, ms = 300): T {
  const [debounced, setV] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setV(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  return debounced;
}
