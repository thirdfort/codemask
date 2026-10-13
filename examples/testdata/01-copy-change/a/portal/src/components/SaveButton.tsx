import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";

type SaveButtonProps = {
  saving: boolean;
  onSave: () => void;
};

export function SaveButton({ saving, onSave }: SaveButtonProps) {
  return (
    <Button aria-label="Save" disabled={saving} onClick={onSave}>
      {saving ? <Spinner /> : null}
      Save
    </Button>
  );
}
