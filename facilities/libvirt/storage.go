// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License. See LICENSE.
//
// Adapted from the virtdeploy package in github.com/microsoft/trident (MIT).

package libvirt

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	golibvirt "github.com/digitalocean/go-libvirt"
	"libvirt.org/go/libvirtxml"
)

var qcow2Magic = []byte{'Q', 'F', 'I', 0xfb}

// inspectImageHeader classifies a disk image from its first bytes: qcow2
// images are recognised by their magic and report their virtual size; anything
// else is a raw image whose capacity is its file size.
func inspectImageHeader(r io.ReaderAt, size int64) (format string, capacity uint64, err error) {
	if size <= 0 {
		return "", 0, fmt.Errorf("image is empty")
	}
	hdr := make([]byte, 32)
	n, err := r.ReadAt(hdr, 0)
	if err != nil && err != io.EOF {
		return "", 0, fmt.Errorf("read image header: %w", err)
	}
	hdr = hdr[:n]
	if len(hdr) >= 32 && string(hdr[:4]) == string(qcow2Magic) {
		virt := binary.BigEndian.Uint64(hdr[24:32])
		if virt == 0 {
			return "", 0, fmt.Errorf("qcow2 image reports a zero virtual size")
		}
		return formatQcow2, virt, nil
	}
	return formatRaw, uint64(size), nil
}

type imageInfo struct {
	path     string
	format   string
	capacity uint64 // virtual size in bytes
	size     uint64 // bytes to upload
}

func inspectImage(path string) (imageInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return imageInfo{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return imageInfo{}, err
	}
	if !st.Mode().IsRegular() {
		return imageInfo{}, fmt.Errorf("%s is not a regular file", path)
	}
	format, capacity, err := inspectImageHeader(f, st.Size())
	if err != nil {
		return imageInfo{}, fmt.Errorf("%s: %w", path, err)
	}
	return imageInfo{path: path, format: format, capacity: capacity, size: uint64(st.Size())}, nil
}

// ctxReader makes a long upload stop at the next read once ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// ensurePool returns the run's directory storage pool. The pool is created
// and started the first time; a pool that already exists is only accepted when
// this client created it earlier or the run was taken over explicitly through
// [Options.RunID]. Callers hold the client lock.
func (c *Client) ensurePool(ctx context.Context) (golibvirt.StoragePool, error) {
	name := c.sc.poolName()
	pool, err := c.lv.StoragePoolLookupByName(name)
	if err == nil {
		if !c.poolReady && !c.adopted {
			return pool, fmt.Errorf("storage pool %s already exists and this client did not create it (set Options.RunID to take over an existing run)", name)
		}
		active, err := c.lv.StoragePoolIsActive(pool)
		if err != nil {
			return pool, fmt.Errorf("check storage pool %s: %w", name, err)
		}
		if active == 0 {
			if err := c.lv.StoragePoolCreate(pool, 0); err != nil {
				return pool, fmt.Errorf("start storage pool %s: %w", name, err)
			}
		}
		c.poolReady = true
		return pool, nil
	}
	if !hasLibvirtCode(err, golibvirt.ErrNoStoragePool) {
		return pool, fmt.Errorf("look up storage pool %s: %w", name, err)
	}
	if err := ctx.Err(); err != nil {
		return pool, err
	}

	spool := libvirtxml.StoragePool{
		Name: name,
		Type: "dir",
		Target: &libvirtxml.StoragePoolTarget{
			Path:        c.sc.poolPath(),
			Permissions: &libvirtxml.StoragePoolTargetPermissions{Owner: "-1", Group: "-1", Mode: "0755"},
		},
	}
	xml, err := spool.Marshal()
	if err != nil {
		return pool, fmt.Errorf("marshal storage pool XML: %w", err)
	}
	if pool, err = c.lv.StoragePoolDefineXML(xml, 0); err != nil {
		return pool, fmt.Errorf("define storage pool %s: %w", name, err)
	}
	if err := c.startNewPool(pool); err != nil {
		// Leave nothing half-created behind.
		_ = c.lv.StoragePoolDestroy(pool)
		_ = c.lv.StoragePoolDelete(pool, 0)
		_ = c.lv.StoragePoolUndefine(pool)
		return pool, err
	}
	c.poolReady = true
	c.log("created storage pool %s at %s", name, c.sc.poolPath())
	return pool, nil
}

func (c *Client) startNewPool(pool golibvirt.StoragePool) error {
	if err := c.lv.StoragePoolBuild(pool, 0); err != nil {
		return fmt.Errorf("build storage pool %s: %w", pool.Name, err)
	}
	if err := c.lv.StoragePoolCreate(pool, 0); err != nil {
		return fmt.Errorf("start storage pool %s: %w", pool.Name, err)
	}
	if err := c.lv.StoragePoolSetAutostart(pool, 1); err != nil {
		return fmt.Errorf("set storage pool %s to autostart: %w", pool.Name, err)
	}
	return nil
}

// createVolume creates a volume in pool and returns its host path. With img
// nil the volume is blank; otherwise the image is uploaded into it. capacity
// is the final virtual size in bytes: when it exceeds the image's size the
// volume is enlarged after the upload.
func (c *Client) createVolume(ctx context.Context, pool golibvirt.StoragePool, name, format string, capacity uint64, img *imageInfo) (string, error) {
	initial := capacity
	if img != nil {
		initial = img.capacity
	}
	vol := libvirtxml.StorageVolume{
		Name:     name,
		Capacity: &libvirtxml.StorageVolumeSize{Unit: "B", Value: initial},
		Target: &libvirtxml.StorageVolumeTarget{
			Format:      &libvirtxml.StorageVolumeTargetFormat{Type: format},
			Permissions: &libvirtxml.StorageVolumeTargetPermissions{Mode: "0644"},
		},
	}
	if format == formatRaw {
		// Keep raw volumes sparse instead of preallocating the full capacity.
		vol.Allocation = &libvirtxml.StorageVolumeSize{Unit: "B", Value: 0}
	}
	xml, err := vol.Marshal()
	if err != nil {
		return "", fmt.Errorf("marshal volume XML: %w", err)
	}
	lvVol, err := c.lv.StorageVolCreateXML(pool, xml, 0)
	if err != nil {
		return "", fmt.Errorf("create volume %s: %w", name, err)
	}

	if img != nil {
		f, err := os.Open(img.path)
		if err != nil {
			return "", fmt.Errorf("open image: %w", err)
		}
		defer f.Close()
		c.log("uploading %s (%d bytes, %s) to volume %s", img.path, img.size, img.format, name)
		if err := c.lv.StorageVolUpload(lvVol, ctxReader{ctx, f}, 0, img.size, 0); err != nil {
			return "", fmt.Errorf("upload %s to volume %s: %w", img.path, name, err)
		}
		if capacity > img.capacity {
			if err := c.lv.StorageVolResize(lvVol, capacity, 0); err != nil {
				return "", fmt.Errorf("enlarge volume %s to %d bytes: %w", name, capacity, err)
			}
		}
	}

	path, err := c.lv.StorageVolGetPath(lvVol)
	if err != nil {
		return "", fmt.Errorf("get path of volume %s: %w", name, err)
	}
	return path, nil
}

// deleteVolumeByName removes a volume from pool; a missing volume is fine.
func (c *Client) deleteVolumeByName(pool golibvirt.StoragePool, name string) error {
	vol, err := c.lv.StorageVolLookupByName(pool, name)
	if err != nil {
		if hasLibvirtCode(err, golibvirt.ErrNoStorageVol) {
			return nil
		}
		return fmt.Errorf("look up volume %s: %w", name, err)
	}
	if err := c.lv.StorageVolDelete(vol, 0); err != nil && !hasLibvirtCode(err, golibvirt.ErrNoStorageVol) {
		return fmt.Errorf("delete volume %s: %w", name, err)
	}
	return nil
}
