#include <stdio.h>
#include <time.h>
#include <gst/app/gstappsrc.h>

#include "gst.h"

#define GST_RTP_EVENT_RETRANSMISSION_REQUEST "GstRTPRetransmissionRequest"

// Internals (snake_case)

void stop_pipeline(GstElement* pipeline) {
    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    gst_element_set_state(pipeline, GST_STATE_NULL);
    gst_object_unref(pipeline);

    goDeletePipeline(id);
}


static gboolean bus_callback(GstBus *bus, GstMessage *msg, gpointer data)
{
    GstElement* pipeline = (GstElement*) data;
    char *id = gst_element_get_name(pipeline);

    switch (GST_MESSAGE_TYPE(msg))
    {
    case GST_MESSAGE_EOS: {
        stop_pipeline(pipeline);
        break;
    }
    case GST_MESSAGE_ERROR:
    {
        GError *error;

        char msgBuf[100];
        sprintf(msgBuf, "ERR [gst.c] from element %d: %s\n",GST_OBJECT_NAME (msg->src), error->message);
        goPipelineLog(id, msgBuf, 1);

        g_error_free(error);

        stop_pipeline(pipeline);
        break;
    }
    default:
        //g_print("got message %s\n", gst_message_type_get_name (GST_MESSAGE_TYPE (msg)));
        break;
    }

    return TRUE;
}

GstFlowReturn new_audio_sample_callback(GstElement *object, gpointer data)
{
    GstSample *sample = NULL;
    GstBuffer *buffer = NULL;
    gpointer copy = NULL;
    gsize copy_size = 0;
    GstElement *pipeline = (GstElement*) data;

    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    g_signal_emit_by_name(object, "pull-sample", &sample);
    if (sample)
    {
        buffer = gst_sample_get_buffer(sample);
        if (buffer)
        {
            gst_buffer_extract_dup(buffer, 0, gst_buffer_get_size(buffer), &copy, &copy_size);
            goWriteAudio(id, copy, copy_size, GST_BUFFER_PTS(buffer));
        }
        gst_sample_unref(sample);
    }

    return GST_FLOW_OK;
}

GstFlowReturn new_video_sample_callback(GstElement *object, gpointer data)
{
    GstSample *sample = NULL;
    GstBuffer *buffer = NULL;
    gpointer copy = NULL;
    gsize copy_size = 0;
    GstElement *pipeline = (GstElement*) data;

    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    g_signal_emit_by_name(object, "pull-sample", &sample);
    if (sample)
    {
        buffer = gst_sample_get_buffer(sample);
        if (buffer)
        {
            gst_buffer_extract_dup(buffer, 0, gst_buffer_get_size(buffer), &copy, &copy_size);
            goWriteVideo(id, copy, copy_size, GST_BUFFER_PTS(buffer));
        }
        gst_sample_unref(sample);
    }

    return GST_FLOW_OK;
}

// TODO use <gst/video/video.h> implementation
gboolean gst_event_is (GstEvent * event, const gchar * name)
{
  const GstStructure *s;

  g_return_val_if_fail (event != NULL, FALSE);

  if (GST_EVENT_TYPE (event) != GST_EVENT_CUSTOM_UPSTREAM)
    return FALSE;               /* Not a force key unit event */

  s = gst_event_get_structure (event);
  if (s == NULL || !gst_structure_has_name (s, name))
    return FALSE;

  return TRUE;
}

// API: functions called from Go (camelCased)

GMainLoop *gstreamer_main_loop = NULL;

void gstStartMainLoop(void)
{
    // run loop
    gstreamer_main_loop = g_main_loop_new(NULL, FALSE);
    g_main_loop_run(gstreamer_main_loop);
}

GstElement *gstParsePipeline(char *pipelineStr, char *id)
{    
    gst_init(NULL, NULL);
    gst_debug_set_active(TRUE);

    GError *error = NULL;
    GstElement *pipeline = gst_parse_launch(pipelineStr, &error);

    // use element name to store id (used when C calls go on new samples to reference what pipeline is involved)
    gst_element_set_name(pipeline, id);

    return pipeline;
}

void gstStartPipeline(GstElement *pipeline)
{
    GstBus *bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline));
    gst_bus_add_watch(bus, bus_callback, pipeline);
    gst_object_unref(bus);
    // src
    GstElement *video_src = gst_bin_get_by_name(GST_BIN(pipeline), "video_src");
    GstPad *video_src_pad = gst_element_get_static_pad(video_src, "src");
    gst_object_unref(video_src);
    gst_object_unref(video_src_pad);
    // sinks
    GstElement *audio_sink = gst_bin_get_by_name(GST_BIN(pipeline), "audio_sink");
    GstElement *video_sink = gst_bin_get_by_name(GST_BIN(pipeline), "video_sink");
    g_object_set(audio_sink, "emit-signals", TRUE, NULL);
    g_signal_connect(audio_sink, "new-sample", G_CALLBACK(new_audio_sample_callback), pipeline);
    gst_object_unref(audio_sink);
    g_object_set(video_sink, "emit-signals", TRUE, NULL);
    g_signal_connect(video_sink, "new-sample", G_CALLBACK(new_video_sample_callback), pipeline);
    gst_object_unref(video_sink);
    // buffer request pad
    GstElement *audio_buffer = gst_bin_get_by_name(GST_BIN(pipeline), "audio_buffer");
    GstElement *video_buffer = gst_bin_get_by_name(GST_BIN(pipeline), "video_buffer");

    // TODO push_rtcp does not work
    // TODO deprecated gst_element_get_request_pad https://gitlab.freedesktop.org/gstreamer/gst-docs/-/merge_requests/152
    // update when GStreamer 1.20 is out
    // GstPad *audio_rtcp_pad = gst_element_get_request_pad (audio_buffer, "sink_rtcp");
    // GstPad *video_rtcp_pad = gst_element_get_request_pad (video_buffer, "sink_rtcp");
    // gst_pad_activate_mode (audio_rtcp_pad, GST_PAD_MODE_PULL, TRUE);
    // gst_pad_activate_mode (video_rtcp_pad, GST_PAD_MODE_PULL, TRUE);
    // gst_object_unref(audio_buffer);
    // gst_object_unref(video_buffer);
    // gst_object_unref(audio_rtcp_pad);
    // gst_object_unref(video_rtcp_pad);

    gst_element_set_state(pipeline, GST_STATE_PLAYING);
}

void gstStopPipeline(GstElement *pipeline)
{
    // query GstStateChangeReturn within 0.1s, if GST_STATE_CHANGE_ASYNC, sending an EOS will fail main loop
    GstStateChangeReturn changeReturn = gst_element_get_state(pipeline, NULL, NULL, 100000000);

    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    if(changeReturn == GST_STATE_CHANGE_ASYNC) {
        // force stop
        stop_pipeline(pipeline);
    } else {
        // gracefully stops media recording
        gst_element_send_event(pipeline, gst_event_new_eos());
    }
}

void gstPushBuffer(char *srcname, GstElement *pipeline, void *buffer, int len)
{
    GstElement *src = gst_bin_get_by_name(GST_BIN(pipeline), srcname);
    
    if (src != NULL)
    {
        gpointer p = g_memdup(buffer, len);
        GstBuffer *buffer = gst_buffer_new_wrapped(p, len);
        gst_app_src_push_buffer(GST_APP_SRC(src), buffer);
        gst_object_unref(src);
    }
}

// float get/set

float gstGetPropFloat(GstElement *pipeline, char *name, char *prop) {
    GstElement* el;
    gfloat value;
 
    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_get(el, prop, &value, NULL);
        gst_object_unref(el);
    }

    return value;
}

void gstSetPropFloat(GstElement *pipeline, char *name, char *prop, float value)
{
    GstElement* el;

    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_set(el, prop, value, NULL);
        gst_object_unref(el);
    }
}

// double get/set

double gstGetPropDouble(GstElement *pipeline, char *name, char *prop) {
    GstElement* el;
    gdouble value;
 
    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_get(el, prop, &value, NULL);
        gst_object_unref(el);
    }

    return value;
}

void gstSetPropDouble(GstElement *pipeline, char *name, char *prop, double value)
{
    GstElement* el;

    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_set(el, prop, value, NULL);
        gst_object_unref(el);
    }
}

// int get/set

gint gstGetPropInt(GstElement *pipeline, char *name, char *prop) {
    GstElement* el;
    gint value;
 
    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_get(el, prop, &value, NULL);
        gst_object_unref(el);
    }

    return value;
}

void gstSetPropInt(GstElement *pipeline, char *name, char *prop, gint value)
{
    GstElement* el;

    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_set(el, prop, value, NULL);
        gst_object_unref(el);
    }
}

// uint64 get/set

guint64 gstGetPropUint64(GstElement *pipeline, char *name, char *prop) {
    GstElement* el;
    guint64 value;
 
    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_get(el, prop, &value, NULL);
        gst_object_unref(el);
    }

    return value;
}

void gstSetPropUint64(GstElement *pipeline, char *name, char *prop, guint64 value)
{
    GstElement* el;

    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_set(el, prop, value, NULL);
        gst_object_unref(el);
    }
}

// char* get/set

void gstSetPropString(GstElement *pipeline, char *name, char *prop, char *value)
{
    GstElement* el;

    el = gst_bin_get_by_name(GST_BIN(pipeline), name);
    
    if(el) {
        g_object_set(el, prop, value, NULL);
        gst_object_unref(el);
    }
}