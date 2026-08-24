import { Transaction, WalletData } from "../api";
import { useConsoleQuery } from "./use-api";

/**
 * Shared wallet reads for the 充值 / 账单 pages: wallet balance plus the
 * full transaction list (recharge records are filtered client-side).
 */
export function useWalletData() {
  const wallet = useConsoleQuery<WalletData>("/wallet");
  const tx = useConsoleQuery<{ data: Transaction[] }>("/wallet/transactions");
  const txs = tx.data?.data ?? [];
  const topups = txs.filter((t) => t.type === "topup");

  return {
    wallet: wallet.data,
    topups,
    isLoading: wallet.isLoading || tx.isLoading,
    isError: wallet.isError || tx.isError,
    errorMessage: wallet.error || tx.error,
    refetch: () => {
      wallet.refetch();
      tx.refetch();
    },
    txRefetch: tx.refetch,
  };
}
