"use client";

import { Input } from "@/components/ui/input";
import { useTranslations } from "next-intl";
import type { CapacityFieldKey, DraftModel } from "./_shared";

function capacityLabel(field: CapacityFieldKey, t: ReturnType<typeof useTranslations>): string {
  if (field === "context_length") return t("settings.model_context_window");
  if (field === "max_tokens") return t("settings.model_max_tokens");
  if (field === "input_price") return t("settings.model_input_price");
  return t("settings.model_output_price");
}

function capacityUnitHint(field: CapacityFieldKey, t: ReturnType<typeof useTranslations>): string | undefined {
  if (field === "input_price" || field === "output_price") {
    return t("settings.model_price_unit");
  }
  return undefined;
}

export function CapacityField({
  m,
  field,
  fieldText,
  setField,
  settleField,
  placeholder,
  disabled,
  t,
}: {
  m: DraftModel;
  field: CapacityFieldKey;
  fieldText: (m: DraftModel, field: CapacityFieldKey) => string;
  setField: (m: DraftModel, field: CapacityFieldKey, raw: string) => void;
  settleField: (m: DraftModel, field: CapacityFieldKey) => void;
  placeholder: string;
  disabled?: boolean;
  t: ReturnType<typeof useTranslations>;
}) {
  return (
    <label className="space-y-1">
      <span
        className="text-[11px] text-muted-foreground"
        title={capacityUnitHint(field, t)}
      >
        {capacityLabel(field, t)}
      </span>
      <Input
        className="h-8 text-xs"
        inputMode="numeric"
        placeholder={placeholder}
        value={fieldText(m, field)}
        disabled={disabled}
        onChange={(e) => setField(m, field, e.target.value)}
        onBlur={() => settleField(m, field)}
      />
    </label>
  );
}
