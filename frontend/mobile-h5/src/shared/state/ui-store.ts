import { create } from 'zustand';
import type { BottomSheetKey } from '../../domain';

type AuctionUIState = {
  activeSheet: BottomSheetKey | null;
  paymentConfirmOpen: boolean;
  setActiveSheet: (sheet: BottomSheetKey | null) => void;
  setPaymentConfirmOpen: (open: boolean) => void;
};

export const useAuctionUIStore = create<AuctionUIState>((set) => ({
  activeSheet: null,
  paymentConfirmOpen: false,
  setActiveSheet: (activeSheet) => set({ activeSheet }),
  setPaymentConfirmOpen: (paymentConfirmOpen) => set({ paymentConfirmOpen })
}));
