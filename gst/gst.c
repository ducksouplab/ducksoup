#include <stdio.h>
#include <time.h>
#include <gst/app/gstappsrc.h>
#include <gst/app/gstappsink.h>
#include <gst/video/video-event.h>

#include "gst.h"

#define GST_RTP_EVENT_RETRANSMISSION_REQUEST "GstRTPRetransmissionRequest"

// Internals (snake_case)

void stop_pipeline(GstElement* pipeline) {
    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    gst_element_set_state(pipeline, GST_STATE_NULL);
    gst_object_unref(pipeline);

    goDeletePipeline(id);
    g_free(id);
}


static gboolean bus_callback(GstBus *bus, GstMessage *msg, gpointer data)
{
    GstElement* pipeline = (GstElement*) data;
    char *id = gst_element_get_name(pipeline);

    // https://gstreamer.freedesktop.org/documentation/gstreamer/gstmessage.html?gi-language=c
    switch (GST_MESSAGE_TYPE(msg))
    {
    case GST_MESSAGE_EOS: {
        stop_pipeline(pipeline);
        break;
    }
    case GST_MESSAGE_LATENCY: {
        gst_bin_recalculate_latency(GST_BIN(pipeline));
        break;
	}
    // case GST_MESSAGE_WARNING: {
    //     GError *error;
    //     gst_message_parse_warning(msg, &error, NULL);
    //     goLogError(id, error->message, GST_OBJECT_NAME (msg->src));
    //     g_error_free(error);
    //     break;
	// }
    case GST_MESSAGE_ERROR:
    {
        GError *error;
        g_print(">>>2");

        // uncomment if debug info is used 
        // gchar *debug;
        // gst_message_parse_error(msg, &error, &debug);
        // g_free(debug);

        gst_message_parse_error(msg, &error, NULL);

        goLogError(id, error->message, GST_OBJECT_NAME (msg->src));
        stop_pipeline(pipeline);

        g_error_free(error);
        break;
    }
    default:
        g_print(">>>3 got message %s\n", gst_message_type_get_name (GST_MESSAGE_TYPE (msg)));
        break;
    }

    g_free(id);
    return TRUE;
}

GstPadProbeReturn videosrc_callback(GstPad *pad, GstPadProbeInfo *info, gpointer data)
{
    
    GstEvent* event = gst_pad_probe_info_get_event(info); // free?
    gboolean fku = gst_video_event_is_force_key_unit(event);
    // g_print("event is forced key unit %d\n", fku);

    if(fku)
    {
        GstElement *pipeline = (GstElement*) data;
        char *id = gst_element_get_name(pipeline);
        goRequestKeyFrame(id);
        g_free(id);
    }
    
}

GstFlowReturn audio_rtp_sink_callback(GstElement *sink, gpointer data)
{
    GstSample *sample;
    GstBuffer *buffer;
    GstElement *pipeline = (GstElement*) data;

    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    sample = gst_app_sink_pull_sample((GstAppSink*) sink);
    if (sample)
    {
        buffer = gst_sample_get_buffer(sample);
        if (buffer)
        {
            GstMapInfo map;
        	gst_buffer_map(buffer, &map, GST_MAP_READ);
            goWriteAudio(id, map.data, map.size);
            gst_buffer_unmap(buffer, &map);
        }
        gst_sample_unref(sample);
    }

    g_free(id);
    return GST_FLOW_OK;
}

GstFlowReturn video_rtp_sink_callback(GstElement *sink, gpointer data)
{
    GstSample *sample;
    GstBuffer *buffer; // free?
    GstElement *pipeline = (GstElement*) data;

    // use previously set name as id
    char *id = gst_element_get_name(pipeline);

    sample = gst_app_sink_pull_sample((GstAppSink*) sink);
    if (sample)
    {
        buffer = gst_sample_get_buffer(sample);
        if (buffer)
        {
            GstMapInfo map;
            gst_buffer_map(buffer, &map, GST_MAP_READ);
            goWriteVideo(id, map.data, map.size);
            gst_buffer_unmap(buffer, &map);
        }
        gst_sample_unref(sample);
    }

    g_free(id);
    return GST_FLOW_OK;
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

void gstStartPipeline(GstElement *pipeline, gboolean audioOnly)
{
    GstBus *bus = gst_pipeline_get_bus(GST_PIPELINE(pipeline));
    gst_bus_add_watch(bus, bus_callback, pipeline);
    gst_object_unref(bus);

    // audio sink configuration
    GstElement *audio_rtp_sink = gst_bin_get_by_name(GST_BIN(pipeline), "audio_rtp_sink");
    g_object_set(audio_rtp_sink, "emit-signals", TRUE, NULL);
    g_signal_connect(audio_rtp_sink, "new-sample", G_CALLBACK(audio_rtp_sink_callback), pipeline);
    gst_object_unref(audio_rtp_sink);

    if(!audioOnly) {
        // video sink configuration
        GstElement *video_rtp_sink = gst_bin_get_by_name(GST_BIN(pipeline), "video_rtp_sink");
        g_object_set(video_rtp_sink, "emit-signals", TRUE, NULL);
        g_signal_connect(video_rtp_sink, "new-sample", G_CALLBACK(video_rtp_sink_callback), pipeline);
        gst_object_unref(video_rtp_sink);
    }

    gst_element_set_state(pipeline, GST_STATE_PLAYING);

    GstElement *videosrc = gst_bin_get_by_name(GST_BIN(pipeline), "video_rtp_src");
    if (videosrc != NULL)
    {
        GstPad *pad = gst_element_get_static_pad(videosrc, "src");
        gst_pad_add_probe(pad,GST_PAD_PROBE_TYPE_EVENT_UPSTREAM, videosrc_callback, pipeline, NULL);
        gst_object_unref(videosrc);
    }

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

    g_free(id);
}

void gstSrcPush(GstElement *pipeline, char *srcname, void *buffer, int len)
{
    GstElement *src = gst_bin_get_by_name(GST_BIN(pipeline), srcname);
    
    if (src != NULL)
    {
        GstBuffer *b = gst_buffer_new_wrapped(buffer, len);
        gst_app_src_push_buffer(GST_APP_SRC(src), b);
        gst_object_unref(src);
    }
}

void gstSendPLI(GstElement *pipeline)
{
    gst_element_send_event(pipeline, gst_video_event_new_upstream_force_key_unit(GST_CLOCK_TIME_NONE, TRUE, 0));
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
    // printf("gstGetPropUint64: %s:%s:%s\n", name, prop, value);
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