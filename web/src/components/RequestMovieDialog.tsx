import { useState } from "react";
import { Loader2 } from "lucide-react";
import type { RequestMediaDetail } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import RequestQualityPicker, { type QualityProfileChoice } from "@/components/RequestQualityPicker";
import { useCreateMediaRequest, useRequestProfileOptions } from "@/hooks/queries/useRequests";
import { requestInputFromMediaResult } from "@/lib/mediaRequests";

/**
 * Requests a movie, with the quality profile it should download with. Only
 * used when the server offers a choice: with one profile, or with the picker
 * turned off, the Request button submits in one click as before.
 */
export default function RequestMovieDialog({
  open,
  onOpenChange,
  item,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  item: RequestMediaDetail;
}) {
  const options = useRequestProfileOptions(item.media_type, open);
  const createRequest = useCreateMediaRequest();
  const [choice, setChoice] = useState<QualityProfileChoice | null>(null);

  const close = (next: boolean) => {
    if (!next) setChoice(null);
    onOpenChange(next);
  };
  const submit = () => {
    createRequest.mutate(
      {
        ...requestInputFromMediaResult(item),
        ...(choice
          ? {
              quality_profile_id: choice.profileId,
              quality_profile_connection_id: choice.connectionId,
            }
          : {}),
      },
      { onSuccess: () => close(false) },
    );
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Request movie</DialogTitle>
          <DialogDescription>Request {item.title} for the library.</DialogDescription>
        </DialogHeader>

        <RequestQualityPicker
          options={options.data}
          value={choice}
          onChange={setChoice}
          disabled={createRequest.isPending}
        />

        <DialogFooter>
          <Button variant="outline" onClick={() => close(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={createRequest.isPending} className="gap-2">
            {createRequest.isPending && <Loader2 className="size-4 animate-spin" />}
            Request movie
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
