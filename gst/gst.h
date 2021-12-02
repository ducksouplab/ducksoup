#ifndef GST_H
#define GST_H

#include <glib.h>
#include <gst/gst.h>
#include <stdint.h>
#include <stdlib.h>

extern void goWriteAudio(char *id, void *buffer, int bufferLen, int pts);
extern void goWriteVideo(char *id, void *buffer, int bufferLen, int pts);
extern void goPLIRequest(char *id);
extern void goDeletePipeline(char *id);
extern void goPipelineLog(char *id, char *msg, int isError);
extern void goDebugLog(int level, char *file, char *function,int line, char *msg);

void gstStartMainLoop(void);
GstElement *gstParsePipeline(char *pipelineStr, char *id);
void gstStartPipeline(GstElement *pipeline);
void gstStopPipeline(GstElement *pipeline);
void gstPushBuffer(char *src, GstElement *pipeline, void *buffer, int len);
float gstGetPropFloat(GstElement *pipeline, char *elName, char *elProp);
void gstSetPropFloat(GstElement *pipeline, char *elName, char *elProp, float elValue);
gint gstGetPropInt(GstElement *pipeline, char *elName, char *elProp);
void gstSetPropInt(GstElement *pipeline, char *elName, char *elProp, gint elValue);
// void gstPushRTCPBuffer(char *name, GstElement *pipeline, void *buffer, int len);

#endif